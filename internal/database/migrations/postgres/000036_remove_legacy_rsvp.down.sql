DROP TABLE invitation_messages;

ALTER TABLE events ADD COLUMN share_token TEXT;
UPDATE events SET share_token = 'rollback-' || id;
ALTER TABLE events ALTER COLUMN share_token SET NOT NULL;
ALTER TABLE events ADD CONSTRAINT events_share_token_key UNIQUE (share_token);
CREATE INDEX idx_events_share_token ON events(share_token);
ALTER TABLE events ADD COLUMN contact_requirement TEXT NOT NULL DEFAULT 'email_or_phone';
ALTER TABLE events ADD COLUMN max_capacity INTEGER;
ALTER TABLE events ADD COLUMN waitlist_enabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE events ADD COLUMN comments_enabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE event_series ADD COLUMN contact_requirement TEXT NOT NULL DEFAULT 'email';
ALTER TABLE event_series ADD COLUMN max_capacity INTEGER;

CREATE TABLE attendees (
    id TEXT PRIMARY KEY, event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    name TEXT NOT NULL, email TEXT, phone TEXT,
    rsvp_status TEXT NOT NULL DEFAULT 'pending' CHECK(rsvp_status IN ('pending','attending','maybe','declined','waitlisted')),
    rsvp_token TEXT NOT NULL UNIQUE, contact_method TEXT NOT NULL DEFAULT 'email'
        CHECK(contact_method IN ('email','sms')), dietary_notes TEXT NOT NULL DEFAULT '',
    plus_ones INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
    import_source TEXT
);
CREATE INDEX idx_attendees_event_id ON attendees(event_id);
CREATE INDEX idx_attendees_rsvp_token ON attendees(rsvp_token);
CREATE INDEX idx_attendees_email ON attendees(email);
CREATE INDEX idx_attendees_rsvp_status ON attendees(rsvp_status);

CREATE TABLE attendee_answers (
    id TEXT PRIMARY KEY, attendee_id TEXT NOT NULL REFERENCES attendees(id) ON DELETE CASCADE,
    question_id TEXT NOT NULL REFERENCES event_questions(id), answer TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL, updated_at TEXT NOT NULL, UNIQUE(attendee_id, question_id)
);
CREATE INDEX idx_attendee_answers_attendee_id ON attendee_answers(attendee_id);

CREATE TABLE event_comments (
    id TEXT PRIMARY KEY, event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    attendee_id TEXT NOT NULL REFERENCES attendees(id) ON DELETE CASCADE,
    author_name TEXT NOT NULL, body TEXT NOT NULL, created_at TEXT NOT NULL
);
CREATE INDEX idx_event_comments_event_id ON event_comments(event_id);
CREATE INDEX idx_event_comments_attendee_id ON event_comments(attendee_id);
CREATE INDEX idx_event_comments_created_at ON event_comments(event_id, created_at);

CREATE TABLE messages (
    id TEXT PRIMARY KEY, event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    sender_type TEXT NOT NULL CHECK(sender_type IN ('organizer','attendee')), sender_id TEXT NOT NULL,
    recipient_type TEXT NOT NULL CHECK(recipient_type IN ('organizer','attendee','group')),
    recipient_id TEXT NOT NULL, subject TEXT NOT NULL DEFAULT '', body TEXT NOT NULL,
    read_at TEXT, created_at TEXT NOT NULL
);
CREATE INDEX idx_messages_event_id ON messages(event_id);
CREATE INDEX idx_messages_sender_id ON messages(sender_id);
CREATE INDEX idx_messages_recipient_id ON messages(recipient_id);

ALTER TABLE notification_log ADD COLUMN attendee_id TEXT REFERENCES attendees(id) ON DELETE SET NULL;
CREATE INDEX idx_notification_log_attendee_id ON notification_log(attendee_id);
DROP INDEX idx_notification_log_invitation_id;
ALTER TABLE notification_log DROP COLUMN invitation_id;
