package instanceconfig

import "context"

// Service provides typed access to instance configuration settings.
type Service struct {
	store *Store
}

// NewService creates a new instance config Service.
func NewService(store *Store) *Service {
	return &Service{store: store}
}

// IsConfigured reports whether the first-run wizard has been completed.
func (s *Service) IsConfigured(ctx context.Context) (bool, error) {
	return s.store.IsConfigured(ctx)
}

// GetSettings returns the full typed settings, falling back to zero values for
// any key that has not been set yet.
func (s *Service) GetSettings(ctx context.Context) (*Settings, error) {
	instance, err := s.store.GetInstance(ctx)
	if err != nil {
		return nil, err
	}
	return &Settings{
		InstanceName:    instance.Name,
		DefaultTimezone: instance.DefaultTimezone,
		AllowSignups:    instance.AllowSignups,
		SupportEmail:    instance.SupportEmail,
		Configured:      instance.SetupCompletedAt != nil,
	}, nil
}

// SaveSettings persists ongoing editable settings. Bootstrap completion is a
// separate one-time transaction and cannot be changed through this method.
func (s *Service) SaveSettings(ctx context.Context, in *Settings) error {
	return s.store.UpdateSettings(ctx, in)
}

// GetPublicConfig returns the non-sensitive subset for the frontend.
func (s *Service) GetPublicConfig(ctx context.Context) (*PublicConfig, error) {
	instance, err := s.store.GetInstance(ctx)
	if err != nil {
		return nil, err
	}
	return &PublicConfig{
		InstanceName:    instance.Name,
		DefaultTimezone: instance.DefaultTimezone,
		AllowSignups:    instance.AllowSignups,
		SupportEmail:    instance.SupportEmail,
	}, nil
}

func boolToString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
