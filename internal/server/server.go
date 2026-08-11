package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/yannkr/openrsvp/internal/auth"
	"github.com/yannkr/openrsvp/internal/config"
	"github.com/yannkr/openrsvp/internal/database"
	"github.com/yannkr/openrsvp/internal/event"
	"github.com/yannkr/openrsvp/internal/eventadmin"
	"github.com/yannkr/openrsvp/internal/feedback"
	"github.com/yannkr/openrsvp/internal/instanceconfig"
	invitationdomain "github.com/yannkr/openrsvp/internal/invitation"
	"github.com/yannkr/openrsvp/internal/invite"
	"github.com/yannkr/openrsvp/internal/notification"
	"github.com/yannkr/openrsvp/internal/notification/templates"
	"github.com/yannkr/openrsvp/internal/question"
	"github.com/yannkr/openrsvp/internal/scheduler"
	"github.com/yannkr/openrsvp/internal/security"
	"github.com/yannkr/openrsvp/internal/stats"
	"github.com/yannkr/openrsvp/internal/suppression"
	"github.com/yannkr/openrsvp/internal/useradmin"
	"github.com/yannkr/openrsvp/internal/webhook"
)

// Server is the main HTTP server for OpenRSVP.
type Server struct {
	cfg                   *config.Config
	db                    database.DB
	logger                zerolog.Logger
	http                  *http.Server
	authHandler           *auth.Handler
	eventHandler          *event.Handler
	seriesHandler         *event.SeriesHandler
	inviteHandler         *invite.Handler
	invitationHandler     *invitationdomain.Handler
	questionHandler       *question.Handler
	feedbackHandler       *feedback.Handler
	reminderHandler       *scheduler.Handler
	webhookHandler        *webhook.Handler
	notifHandler          *notification.Handler
	notifService          *notification.Service
	statsHandler          *stats.Handler
	suppressionHandler    *suppression.Handler
	instanceConfigHandler *instanceconfig.Handler
	userAdminHandler      *useradmin.Handler
	eventAdminHandler     *eventadmin.Handler
	scheduler             *scheduler.Scheduler
	securityMw            *security.Middleware
	uploadsDir            string
	shuttingDown          atomic.Bool
}

// New creates a new Server instance.
func New(cfg *config.Config, db database.DB, logger zerolog.Logger) *Server {
	// Wire up auth layer.
	authStore := auth.NewStore(db)
	authService := auth.NewService(authStore, cfg, logger)
	authHandler := auth.NewHandler(authService, cfg, logger)
	authMiddleware := auth.RequireAuth(authService)

	organizerFromCtx := func(ctx context.Context) (string, bool) {
		org := auth.OrganizerFromContext(ctx)
		if org == nil {
			return "", false
		}
		return org.ID, true
	}

	// Wire up event layer.
	eventStore := event.NewStore(db)
	eventService := event.NewService(eventStore, cfg.DefaultRetentionDays)

	// Wire up co-host store and set it on the event service.
	cohostStore := event.NewCoHostStore(db)
	eventService.SetCoHostStore(cohostStore)

	// Organizer lookup by email for co-host management.
	organizerLookupByEmail := event.OrganizerLookupByEmail(func(ctx context.Context, email string) (string, string, error) {
		org, err := authStore.FindOrganizerByEmail(ctx, email)
		if err != nil {
			return "", "", err
		}
		if org == nil {
			return "", "", nil
		}
		if org.Status != auth.UserStatusActive {
			return "", "", nil
		}
		return org.ID, org.Name, nil
	})

	eventHandler := event.NewHandler(
		eventService, authMiddleware, event.OrganizerFromCtx(organizerFromCtx), logger,
		event.WithCoHostStore(cohostStore),
		event.WithOrganizerLookup(organizerLookupByEmail),
		event.WithMaxCoHosts(cfg.MaxCoHostsPerEvent),
	)

	// Wire up event series layer.
	seriesStore := event.NewSeriesStore(db)
	seriesService := event.NewSeriesService(seriesStore, eventStore, eventService, cfg.DefaultRetentionDays, logger)
	seriesHandler := event.NewSeriesHandler(seriesService, authMiddleware, event.OrganizerFromCtx(organizerFromCtx), logger)

	// checkEventOwner verifies that the given organizer can manage the event
	// (either as owner or co-host).
	// Returns nil if the organizer can manage the event; a non-nil error otherwise.
	checkEventOwner := func(ctx context.Context, eventID, organizerID string) error {
		canManage, err := eventService.CanManageEvent(ctx, eventID, organizerID)
		if err != nil {
			return err
		}
		if !canManage {
			return fmt.Errorf("event not found")
		}
		return nil
	}

	// Ensure uploads directory exists.
	uploadsDir := cfg.UploadsDir
	if err := os.MkdirAll(uploadsDir, 0755); err != nil {
		logger.Error().Err(err).Str("dir", uploadsDir).Msg("failed to create uploads directory")
	}

	// Wire up invite layer (before RSVP since RSVP depends on it).
	inviteStore := invite.NewStore(db)
	inviteService := invite.NewService(inviteStore, uploadsDir)
	inviteHandler := invite.NewHandler(inviteService, authMiddleware, invite.OrganizerFromCtx(organizerFromCtx), uploadsDir, invite.EventOwnershipChecker(checkEventOwner), logger)

	// Wire up question layer.
	questionStore := question.NewStore(db)
	questionService := question.NewService(questionStore)
	questionHandler := question.NewHandler(questionService, authMiddleware, question.OrganizerFromCtx(organizerFromCtx), question.EventOwnershipChecker(checkEventOwner), logger)

	// Wire up webhook layer.
	webhookStore := webhook.NewStore(db)
	webhookService := webhook.NewService(webhookStore, logger, !cfg.IsDevelopment())
	webhookDispatcher := webhook.NewDispatcher(webhookStore, logger)
	webhookHandler := webhook.NewHandler(webhookService, webhookDispatcher, authMiddleware, webhook.OrganizerFromCtx(organizerFromCtx), webhook.EventOwnershipChecker(checkEventOwner), logger)

	// Wire up email suppression / unsubscribe layer (consumed by notification).
	suppressionStore := suppression.NewStore(db)
	suppressionService := suppression.NewService(suppressionStore)
	suppressionHandler := suppression.NewHandler(suppressionService, logger)

	// Wire up notification layer.
	notifRegistry := buildNotificationRegistry(cfg, logger)
	notifService := notification.NewServiceWithOptions(notifRegistry, db, logger, notification.Options{
		BaseURL:             cfg.BaseURL,
		OpenTrackingEnabled: cfg.EmailOpenTrackingEnabled,
		Suppression:         suppressionService,
	})

	// Wire up notification tracking layer.
	trackingService := notification.NewTrackingService(db, logger)
	notifHandler := notification.NewHandler(trackingService, notifService, suppressionService, authMiddleware, notification.OrganizerFromCtx(organizerFromCtx), notification.EventOwnershipChecker(checkEventOwner), logger)

	// Wire up the Gate 2 invitation household and capability boundary. Config
	// loading rejects a missing/short restore key; a directly-constructed invalid
	// config is a programmer error and must not silently start insecure routes.
	invitationStore := invitationdomain.NewStore(db)
	invitationService, err := invitationdomain.NewService(invitationStore,
		cfg.InvitationSecretKey, cfg.BaseURL, cfg.InvitationSessionExpiry,
		cfg.InvitationRecoveryExpiry)
	if err != nil {
		panic("invalid invitation security configuration: " + err.Error())
	}
	if notifRegistry.Has(notification.ChannelEmail) {
		invitationService.SetEmailSender(func(ctx context.Context, eventID, invitationID, to, subject, htmlBody, plainBody string) error {
			return notifService.Send(ctx, eventID, invitationID, notification.ChannelEmail,
				&notification.Message{To: to, Subject: subject, Body: htmlBody, Plain: plainBody})
		})
	}
	invitationHandler := invitationdomain.NewHandler(invitationService, authMiddleware,
		invitationdomain.UserFromCtx(organizerFromCtx),
		invitationdomain.EventAccessChecker(checkEventOwner), cfg.Env == "production", logger)

	// Wire email sending into auth service (breaks circular dep via function).
	if notifRegistry.Has(notification.ChannelEmail) {
		authService.SetEmailSender(func(ctx context.Context, to, subject, htmlBody, plainBody string) error {
			provider, err := notifRegistry.Get(notification.ChannelEmail)
			if err != nil {
				return err
			}
			_, sendErr := provider.Send(ctx, &notification.Message{
				To:      to,
				Subject: subject,
				Body:    htmlBody,
				Plain:   plainBody,
			})
			return sendErr
		})
	}

	// Wire co-host invitation notifications into the event handler.
	if notifRegistry.Has(notification.ChannelEmail) {
		eventHandler.SetNotifyCoHost(func(ctx context.Context, coHostEmail, eventID, addedByOrganizerID string) {
			ev, err := eventService.GetByID(ctx, eventID)
			if err != nil {
				logger.Error().Err(err).Str("event_id", eventID).Msg("cohost notify: failed to get event")
				return
			}

			organizer, err := authStore.FindOrganizerByID(ctx, addedByOrganizerID)
			if err != nil || organizer == nil {
				logger.Error().Err(err).Str("organizer_id", addedByOrganizerID).Msg("cohost notify: failed to get organizer")
				return
			}

			eventDate := ev.EventDate.Format("January 2, 2006 at 3:04 PM")
			location := ev.Location
			if location == "" {
				location = "TBD"
			}
			dashboardURL := cfg.BaseURL + "/events/" + eventID

			htmlBody, plainBody, err := templates.RenderCoHostInvitation(ev.Title, eventDate, location, organizer.Name, dashboardURL)
			if err != nil {
				logger.Error().Err(err).Str("event_id", eventID).Msg("cohost notify: failed to render template")
				return
			}

			if err := notifService.Send(ctx, eventID, addedByOrganizerID, notification.ChannelEmail, &notification.Message{
				To:      coHostEmail,
				Subject: "You've been added as a co-host — " + ev.Title,
				Body:    htmlBody,
				Plain:   plainBody,
			}); err != nil {
				logger.Error().Err(err).Str("cohost_email", coHostEmail).Msg("cohost notify: failed to send email")
			}
		})
	}

	// Wire up feedback layer.
	feedbackSvc := feedback.NewService(cfg.FeedbackGitHubToken, cfg.FeedbackGitHubRepo, cfg.FeedbackEmail)
	if cfg.FeedbackGitHubToken == "" && cfg.FeedbackEmail == "" {
		logger.Warn().Msg("feedback: no channel configured (set FEEDBACK_GITHUB_TOKEN+FEEDBACK_GITHUB_REPO or FEEDBACK_EMAIL) — submissions will be silently discarded")
	}
	if notifRegistry.Has(notification.ChannelEmail) {
		feedbackSvc.SetEmailSender(func(ctx context.Context, to, subject, body, plain string) error {
			provider, err := notifRegistry.Get(notification.ChannelEmail)
			if err != nil {
				return err
			}
			_, sendErr := provider.Send(ctx, &notification.Message{
				To:      to,
				Subject: subject,
				Body:    body,
				Plain:   plain,
			})
			return sendErr
		})
	}
	organizerEmailFromCtx := func(ctx context.Context) (string, bool) {
		org := auth.OrganizerFromContext(ctx)
		if org == nil {
			return "", false
		}
		return org.Email, true
	}
	feedbackHandler := feedback.NewHandler(feedbackSvc, authMiddleware, feedback.OrganizerFromCtx(organizerEmailFromCtx), logger)

	// Wire up security middleware (created early so rate limiters are available
	// for handler constructors that need them).
	secMw := security.NewMiddleware(security.SecurityConfig{
		AuthRateLimit:    10,
		RSVPRateLimit:    30,
		GeneralRateLimit: 200,
		RateWindow:       1 * time.Minute,
		CSRFExcludePaths: []string{
			"/api/v1/invitations/exchange",
			"/api/v1/invitations/recovery/request",
			"/api/v1/invitations/recovery/exchange",
			"/api/v1/invitations/open/inspect",
			"/api/v1/invitations/open/enroll",
			"/api/v1/auth/magic-link",
			"/api/v1/auth/verify",
			"/api/v1/auth/account-invites/accept", // one-time account capability
			"/api/v1/setup/bootstrap",             // one-time env-token-authorized setup
			"/api/v1/feedback/public",             // unauthenticated guest bug reports
			"/api/v1/unsubscribe",                 // token-based email opt-out (no session)
		},
		IsProduction: cfg.Env == "production",
	})

	// Wire up scheduler and reminder layer.
	reminderStore := scheduler.NewReminderStore(db)
	reminderHandler := scheduler.NewHandler(reminderStore, authMiddleware, scheduler.OrganizerFromCtx(organizerFromCtx), scheduler.EventOwnershipChecker(checkEventOwner), logger)

	// Copy invite card design when an event is duplicated.
	eventService.SetOnDuplicate(func(ctx context.Context, srcEventID, newEventID string) {
		card, err := inviteService.GetByEventID(ctx, srcEventID)
		if err != nil || card == nil {
			return
		}
		_, err = inviteService.Save(ctx, newEventID, invite.SaveInviteRequest{
			TemplateID:     card.TemplateID,
			Heading:        card.Heading,
			Body:           card.Body,
			Footer:         card.Footer,
			PrimaryColor:   card.PrimaryColor,
			SecondaryColor: card.SecondaryColor,
			Font:           card.Font,
			CustomData:     card.CustomData,
		})
		if err != nil {
			logger.Error().Err(err).
				Str("src_event_id", srcEventID).
				Str("new_event_id", newEventID).
				Msg("failed to copy invite card during duplication")
		}
	})

	// Create default reminders (1 week and 3 days before) when an event is published.
	eventService.SetOnPublish(func(ctx context.Context, e *event.Event) {
		go webhookDispatcher.Dispatch(context.Background(), e.ID, "event.published", map[string]any{
			"eventId": e.ID,
			"title":   e.Title,
		})

		type defaultReminder struct {
			offset  time.Duration
			message string
		}
		defaults := []defaultReminder{
			{7 * 24 * time.Hour, "Reminder: " + e.Title + " is in 1 week!"},
			{3 * 24 * time.Hour, "Reminder: " + e.Title + " is in 3 days!"},
		}

		now := time.Now().UTC()
		for _, d := range defaults {
			remindAt := e.EventDate.Add(-d.offset)
			if remindAt.Before(now) {
				logger.Debug().
					Str("event_id", e.ID).
					Time("remind_at", remindAt).
					Msg("skipping default reminder (already in the past)")
				continue
			}

			r := &scheduler.Reminder{
				ID:          uuid.Must(uuid.NewV7()).String(),
				EventID:     e.ID,
				RemindAt:    remindAt,
				TargetGroup: "all",
				Message:     d.message,
				Status:      "scheduled",
			}
			if err := reminderStore.Create(ctx, r); err != nil {
				logger.Error().Err(err).
					Str("event_id", e.ID).
					Time("remind_at", remindAt).
					Msg("failed to create default reminder")
				continue
			}
			logger.Info().
				Str("event_id", e.ID).
				Str("reminder_id", r.ID).
				Time("remind_at", remindAt).
				Msg("created default reminder")
		}
	})

	// Send cancellation notifications through isolated invitation destinations.
	if notifRegistry.Has(notification.ChannelEmail) {
		eventService.SetOnCancel(func(ctx context.Context, e *event.Event) {
			go webhookDispatcher.Dispatch(context.Background(), e.ID, "event.cancelled", map[string]any{
				"eventId": e.ID,
				"title":   e.Title,
			})
			if _, err := invitationService.Broadcast(ctx, e.ID, nil, invitationdomain.MessageRequest{
				RecipientGroup: "all",
				Subject:        "Event cancelled — " + e.Title,
				Body:           "This event has been cancelled by the organizer.",
			}); err != nil {
				logger.Error().Err(err).Str("event_id", e.ID).
					Msg("cancellation invitation delivery failed")
			}
		})
	}

	sched := scheduler.New(logger)
	reminderJob := scheduler.NewReminderJob(reminderStore,
		func(ctx context.Context, eventID, group, subject, body string) (int, error) {
			return invitationService.Broadcast(ctx, eventID, nil, invitationdomain.MessageRequest{
				RecipientGroup: group, Subject: subject, Body: body,
			})
		}, logger)
	cleanupJob := scheduler.NewCleanupJob(db, logger)

	// Wire retention warning notifications into the cleanup job.
	if notifRegistry.Has(notification.ChannelEmail) {
		cleanupJob.SetRetentionNotify(func(ctx context.Context, organizerEmail, eventTitle string, expiresAt time.Time) {
			expiresStr := expiresAt.Format("January 2, 2006")
			dashboardURL := cfg.BaseURL + "/events"

			htmlBody, plainBody, err := templates.RenderRetentionWarning(eventTitle, expiresStr, dashboardURL)
			if err != nil {
				logger.Error().Err(err).Str("event_title", eventTitle).Msg("retention warning: failed to render template")
				return
			}

			provider, provErr := notifRegistry.Get(notification.ChannelEmail)
			if provErr != nil {
				logger.Error().Err(provErr).Msg("retention warning: no email provider")
				return
			}

			if _, sendErr := provider.Send(ctx, &notification.Message{
				To:      organizerEmail,
				Subject: "Data Retention Notice — " + eventTitle,
				Body:    htmlBody,
				Plain:   plainBody,
			}); sendErr != nil {
				logger.Error().Err(sendErr).Str("email", organizerEmail).Msg("retention warning: failed to send email")
			}
		})
	}

	// Clean up uploaded files when events are deleted by retention policy.
	cleanupJob.SetOnDeleteEvent(func(eventID string) {
		entries, err := os.ReadDir(uploadsDir)
		if err != nil {
			return
		}
		prefix := eventID + "_"
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasPrefix(entry.Name(), prefix) {
				_ = os.Remove(filepath.Join(uploadsDir, entry.Name()))
				logger.Debug().Str("file", entry.Name()).Msg("cleaned up uploaded file for deleted event")
			}
		}
	})

	sched.Register(reminderJob)
	sched.Register(cleanupJob)

	// Copy invite card when a new series occurrence is generated.
	seriesService.SetOnCreateOccurrence(func(ctx context.Context, seriesID, occurrenceID string) {
		events, err := eventStore.FindBySeriesID(ctx, seriesID)
		if err != nil || len(events) == 0 {
			return
		}
		for _, e := range events {
			if e.ID == occurrenceID {
				continue
			}
			card, err := inviteService.GetByEventID(ctx, e.ID)
			if err != nil || card == nil {
				continue
			}
			_, err = inviteService.Save(ctx, occurrenceID, invite.SaveInviteRequest{
				TemplateID:     card.TemplateID,
				Heading:        card.Heading,
				Body:           card.Body,
				Footer:         card.Footer,
				PrimaryColor:   card.PrimaryColor,
				SecondaryColor: card.SecondaryColor,
				Font:           card.Font,
				CustomData:     card.CustomData,
			})
			if err != nil {
				logger.Error().Err(err).
					Str("series_id", seriesID).
					Str("occurrence_id", occurrenceID).
					Msg("failed to copy invite card for series occurrence")
			}
			break
		}
	})

	// Register series generator background job.
	seriesJob := scheduler.NewSeriesGeneratorJob(seriesService, logger)
	sched.Register(seriesJob)

	// Wire up admin stats layer.
	statsStore := stats.NewStore(db)
	statsService := stats.NewService(statsStore, logger)
	adminMiddleware := auth.RequireAdmin()
	statsHandler := stats.NewHandler(statsService, authMiddleware, adminMiddleware, logger)

	// Persistent user administration and account invitations. Invitation
	// capabilities are delivered only through the configured email provider.
	userAdminStore := useradmin.NewStore(db)
	userAdminService := useradmin.NewService(userAdminStore, authStore, cfg, logger)
	if notifRegistry.Has(notification.ChannelEmail) {
		userAdminService.SetEmailSender(func(ctx context.Context, to, subject, htmlBody, plainBody string) error {
			provider, err := notifRegistry.Get(notification.ChannelEmail)
			if err != nil {
				return err
			}
			_, sendErr := provider.Send(ctx, &notification.Message{To: to, Subject: subject, Body: htmlBody, Plain: plainBody})
			return sendErr
		})
	}
	userAdminHandler := useradmin.NewHandler(userAdminService, cfg, authMiddleware, adminMiddleware, logger)
	eventHandler.SetCoHostSponsor(func(ctx context.Context, ownerUserID, eventID, email string) (bool, error) {
		owner, err := authStore.FindOrganizerByID(ctx, ownerUserID)
		if err != nil || owner == nil {
			return false, err
		}
		return userAdminService.InviteEventCohost(ctx, owner, eventID, email)
	})

	// Wire up instance setup/config layer. DB-backed non-secret overrides
	// (instance name, default timezone, signups, support email) are overlaid
	// on top of the env-derived config at startup.
	instanceConfigStore := instanceconfig.NewStore(db)
	instanceConfigService := instanceconfig.NewService(instanceConfigStore)
	bootstrapService := instanceconfig.NewBootstrapService(instanceConfigStore, authStore, cfg)
	if overrides, err := instanceConfigStore.GetAll(context.Background()); err == nil {
		cfg.ApplyInstanceOverrides(overrides)
	}
	instanceConfigHandler := instanceconfig.NewHandler(instanceConfigService, bootstrapService, cfg, authMiddleware, adminMiddleware, logger)
	eventAdminService := eventadmin.NewService(db, authStore, instanceConfigStore)
	eventAdminHandler := eventadmin.NewHandler(eventAdminService, authMiddleware, adminMiddleware)

	s := &Server{
		cfg:                   cfg,
		db:                    db,
		logger:                logger,
		authHandler:           authHandler,
		eventHandler:          eventHandler,
		seriesHandler:         seriesHandler,
		inviteHandler:         inviteHandler,
		invitationHandler:     invitationHandler,
		questionHandler:       questionHandler,
		feedbackHandler:       feedbackHandler,
		reminderHandler:       reminderHandler,
		webhookHandler:        webhookHandler,
		notifHandler:          notifHandler,
		notifService:          notifService,
		statsHandler:          statsHandler,
		suppressionHandler:    suppressionHandler,
		instanceConfigHandler: instanceConfigHandler,
		userAdminHandler:      userAdminHandler,
		eventAdminHandler:     eventAdminHandler,
		scheduler:             sched,
		securityMw:            secMw,
		uploadsDir:            uploadsDir,
	}

	router := s.routes()

	s.http = &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	return s
}

// Start begins listening and blocks until the provided context is cancelled.
// It performs a graceful shutdown when the context is done.
func (s *Server) Start(ctx context.Context) error {
	s.shuttingDown.Store(false)
	// Start background scheduler.
	s.scheduler.Start(ctx)

	errCh := make(chan error, 1)

	go func() {
		s.logger.Info().Str("addr", s.http.Addr).Msg("server listening")
		if err := s.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err, ok := <-errCh:
		s.shuttingDown.Store(true)
		s.stopBackgroundServices()
		if !ok || err == nil {
			return nil
		}
		return fmt.Errorf("server error: %w", err)
	case <-ctx.Done():
		s.shuttingDown.Store(true)
		s.logger.Info().Msg("shutting down server")
	}

	s.stopBackgroundServices()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.http.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}

	s.logger.Info().Msg("server stopped gracefully")
	return nil
}

func (s *Server) stopBackgroundServices() {
	s.scheduler.Stop()
	s.securityMw.AuthRateLimiter.Stop()
	s.securityMw.RSVPRateLimiter.Stop()
	s.securityMw.GeneralRateLimiter.Stop()
}
