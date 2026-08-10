-- Preserve every Gate 2 child row while rebuilding the legacy-shaped events
-- table. Foreign-key enforcement remains enabled throughout this migration.
CREATE TEMP TABLE gate2_invite_cards AS SELECT * FROM invite_cards;
CREATE TEMP TABLE gate2_reminders AS SELECT * FROM reminders;
CREATE TEMP TABLE gate2_event_questions AS SELECT * FROM event_questions;
CREATE TEMP TABLE gate2_webhooks AS SELECT * FROM webhooks;
CREATE TEMP TABLE gate2_webhook_deliveries AS SELECT * FROM webhook_deliveries;
CREATE TEMP TABLE gate2_event_memberships AS SELECT * FROM event_memberships;
CREATE TEMP TABLE gate2_account_invites AS SELECT * FROM account_invites;
CREATE TEMP TABLE gate2_email_suppressions AS SELECT * FROM email_suppressions;
CREATE TEMP TABLE gate2_unsubscribe_tokens AS SELECT * FROM unsubscribe_tokens;
CREATE TEMP TABLE gate2_open_enrollments AS SELECT * FROM open_enrollments;
CREATE TEMP TABLE gate2_invitations AS SELECT * FROM invitations;
CREATE TEMP TABLE gate2_guests AS SELECT * FROM guests;
CREATE TEMP TABLE gate2_rsvp_responses AS SELECT * FROM rsvp_responses;
CREATE TEMP TABLE gate2_guest_responses AS SELECT * FROM guest_responses;
CREATE TEMP TABLE gate2_invitation_answers AS SELECT * FROM invitation_answers;
CREATE TEMP TABLE gate2_guest_answers AS SELECT * FROM guest_answers;
CREATE TEMP TABLE gate2_invitation_sessions AS SELECT * FROM invitation_sessions;
CREATE TEMP TABLE gate2_invitation_recovery_tokens AS SELECT * FROM invitation_recovery_tokens;
CREATE TEMP TABLE gate2_invitation_recovery_attempts AS SELECT * FROM invitation_recovery_attempts;
CREATE TEMP TABLE gate2_admin_audit_events AS
SELECT id, event_id FROM admin_audit_log WHERE event_id IS NOT NULL;
CREATE TEMP TABLE gate2_notification_log AS
SELECT n.id, n.event_id,
       CASE WHEN i.id IS NULL THEN NULL ELSE i.id END AS invitation_id,
       n.channel, n.provider, n.status, n.error, n.sent_at, n.created_at,
       n.recipient, n.subject, n.message_id, n.delivery_status,
       n.delivered_at, n.opened_at, n.clicked_at, n.bounced_at,
       n.bounce_type, n.complaint_at
FROM notification_log n
LEFT JOIN invitations i ON i.id = 'legacy-invitation:' || n.attendee_id;

-- Leaf-to-root deletion prevents restrictive answer FKs from blocking the
-- parent rebuild. The rows are restored after the new events table exists.
DELETE FROM webhook_deliveries;
DELETE FROM invitation_answers;
DELETE FROM guest_answers;
DELETE FROM guest_responses;
DELETE FROM invitation_sessions;
DELETE FROM invitation_recovery_tokens;
DELETE FROM invitation_recovery_attempts;
DELETE FROM guests;
DELETE FROM rsvp_responses;
DELETE FROM invitations;
DELETE FROM open_enrollments;
DELETE FROM webhooks;
DELETE FROM attendee_answers;
DELETE FROM event_questions;
DELETE FROM invite_cards;
DELETE FROM reminders;
DELETE FROM event_memberships;
DELETE FROM account_invites;
DELETE FROM email_suppressions;
DELETE FROM unsubscribe_tokens;
UPDATE admin_audit_log SET event_id = NULL;

DROP TABLE notification_log;
DROP TABLE event_comments;
DROP TABLE attendee_answers;
DROP TABLE attendees;
DROP TABLE messages;

CREATE TABLE events_next (
    id              TEXT PRIMARY KEY,
    title           TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    event_date      TEXT NOT NULL,
    end_date        TEXT,
    location        TEXT NOT NULL DEFAULT '',
    timezone        TEXT NOT NULL DEFAULT 'America/New_York',
    retention_days  INTEGER NOT NULL DEFAULT 30,
    status          TEXT NOT NULL DEFAULT 'draft'
                    CHECK(status IN ('draft','published','cancelled','archived','retention_warning')),
    show_headcount  INTEGER NOT NULL DEFAULT 0,
    show_guest_list INTEGER NOT NULL DEFAULT 0,
    rsvp_deadline   TEXT,
    series_id       TEXT REFERENCES event_series(id) ON DELETE SET NULL,
    series_index    INTEGER,
    series_override INTEGER NOT NULL DEFAULT 0,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);
INSERT INTO events_next (
    id, title, description, event_date, end_date, location, timezone,
    retention_days, status, show_headcount, show_guest_list, rsvp_deadline,
    series_id, series_index, series_override, created_at, updated_at
)
SELECT id, title, description, event_date, end_date, location, timezone,
       retention_days, status, show_headcount, show_guest_list, rsvp_deadline,
       series_id, series_index, series_override, created_at, updated_at
FROM events;
DROP TABLE events;
ALTER TABLE events_next RENAME TO events;
CREATE INDEX idx_events_status ON events(status);
CREATE INDEX idx_events_event_date ON events(event_date);
CREATE INDEX idx_events_series_id ON events(series_id);

ALTER TABLE event_series DROP COLUMN contact_requirement;
ALTER TABLE event_series DROP COLUMN max_capacity;

INSERT INTO invite_cards SELECT * FROM gate2_invite_cards;
INSERT INTO reminders SELECT * FROM gate2_reminders;
INSERT INTO event_questions SELECT * FROM gate2_event_questions;
INSERT INTO webhooks SELECT * FROM gate2_webhooks;
INSERT INTO webhook_deliveries SELECT * FROM gate2_webhook_deliveries;
INSERT INTO event_memberships SELECT * FROM gate2_event_memberships;
INSERT INTO account_invites SELECT * FROM gate2_account_invites;
INSERT INTO email_suppressions SELECT * FROM gate2_email_suppressions;
INSERT INTO unsubscribe_tokens SELECT * FROM gate2_unsubscribe_tokens;
INSERT INTO open_enrollments SELECT * FROM gate2_open_enrollments;
INSERT INTO invitations SELECT * FROM gate2_invitations;
INSERT INTO guests SELECT * FROM gate2_guests;
INSERT INTO rsvp_responses SELECT * FROM gate2_rsvp_responses;
INSERT INTO guest_responses SELECT * FROM gate2_guest_responses;
INSERT INTO invitation_answers SELECT * FROM gate2_invitation_answers;
INSERT INTO guest_answers SELECT * FROM gate2_guest_answers;
INSERT INTO invitation_sessions SELECT * FROM gate2_invitation_sessions;
INSERT INTO invitation_recovery_tokens SELECT * FROM gate2_invitation_recovery_tokens;
INSERT INTO invitation_recovery_attempts SELECT * FROM gate2_invitation_recovery_attempts;
UPDATE admin_audit_log
SET event_id = (SELECT saved.event_id FROM gate2_admin_audit_events saved
                WHERE saved.id = admin_audit_log.id)
WHERE id IN (SELECT id FROM gate2_admin_audit_events);

CREATE TABLE notification_log (
    id              TEXT PRIMARY KEY,
    event_id        TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    invitation_id   TEXT REFERENCES invitations(id) ON DELETE SET NULL,
    channel         TEXT NOT NULL CHECK(channel IN ('email','sms')),
    provider        TEXT NOT NULL,
    status          TEXT NOT NULL CHECK(status IN ('pending','sent','failed')),
    error           TEXT,
    sent_at         TEXT,
    created_at      TEXT NOT NULL,
    recipient       TEXT NOT NULL DEFAULT '',
    subject         TEXT NOT NULL DEFAULT '',
    message_id      TEXT,
    delivery_status TEXT NOT NULL DEFAULT 'unknown',
    delivered_at    TEXT,
    opened_at       TEXT,
    clicked_at      TEXT,
    bounced_at      TEXT,
    bounce_type     TEXT,
    complaint_at    TEXT
);
INSERT INTO notification_log SELECT * FROM gate2_notification_log;
CREATE INDEX idx_notification_log_event_id ON notification_log(event_id);
CREATE INDEX idx_notification_log_invitation_id ON notification_log(invitation_id);
CREATE INDEX idx_notification_log_status ON notification_log(status);
CREATE INDEX idx_notification_log_message_id ON notification_log(message_id);
CREATE INDEX idx_notification_log_recipient ON notification_log(recipient);

CREATE TABLE invitation_messages (
    id               TEXT PRIMARY KEY,
    event_id         TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    sender_user_id   TEXT REFERENCES users(id) ON DELETE SET NULL,
    recipient_group  TEXT NOT NULL CHECK(recipient_group IN ('all','attending','maybe','declined','pending')),
    subject          TEXT NOT NULL,
    body             TEXT NOT NULL,
    created_at       TEXT NOT NULL
);
CREATE INDEX idx_invitation_messages_event ON invitation_messages(event_id, created_at);

PRAGMA foreign_key_check;
