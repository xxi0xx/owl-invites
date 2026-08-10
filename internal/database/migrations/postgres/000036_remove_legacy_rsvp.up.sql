ALTER TABLE notification_log ADD COLUMN invitation_id TEXT REFERENCES invitations(id) ON DELETE SET NULL;
UPDATE notification_log n
SET invitation_id = i.id
FROM invitations i
WHERE n.attendee_id IS NOT NULL AND i.id = 'legacy-invitation:' || n.attendee_id;
CREATE INDEX idx_notification_log_invitation_id ON notification_log(invitation_id);
DROP INDEX idx_notification_log_attendee_id;
ALTER TABLE notification_log DROP COLUMN attendee_id;

DROP TABLE event_comments;
DROP TABLE attendee_answers;
DROP TABLE attendees;
DROP TABLE messages;

DROP INDEX idx_events_share_token;
ALTER TABLE events DROP COLUMN share_token;
ALTER TABLE events DROP COLUMN contact_requirement;
ALTER TABLE events DROP COLUMN max_capacity;
ALTER TABLE events DROP COLUMN waitlist_enabled;
ALTER TABLE events DROP COLUMN comments_enabled;

ALTER TABLE event_series DROP COLUMN contact_requirement;
ALTER TABLE event_series DROP COLUMN max_capacity;

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
