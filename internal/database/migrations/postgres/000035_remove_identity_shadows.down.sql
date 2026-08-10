CREATE TABLE organizers (
    id         TEXT PRIMARY KEY,
    email      TEXT NOT NULL UNIQUE,
    name       TEXT NOT NULL DEFAULT '',
    timezone   TEXT NOT NULL DEFAULT '',
    is_admin   BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
INSERT INTO organizers (id, email, name, timezone, is_admin, created_at, updated_at)
SELECT id, email, display_name, timezone, instance_role = 'admin', created_at, updated_at
FROM users;
CREATE INDEX idx_organizers_email ON organizers(email);

ALTER TABLE magic_links DROP CONSTRAINT magic_links_user_id_fkey;
ALTER TABLE magic_links RENAME COLUMN user_id TO organizer_id;
ALTER TABLE magic_links ADD CONSTRAINT magic_links_organizer_id_fkey
    FOREIGN KEY (organizer_id) REFERENCES organizers(id);
DROP INDEX idx_magic_links_user_id;
CREATE INDEX idx_magic_links_organizer_id ON magic_links(organizer_id);

ALTER TABLE sessions DROP CONSTRAINT sessions_user_id_fkey;
ALTER TABLE sessions RENAME COLUMN user_id TO organizer_id;
ALTER TABLE sessions ADD CONSTRAINT sessions_organizer_id_fkey
    FOREIGN KEY (organizer_id) REFERENCES organizers(id);
DROP INDEX idx_sessions_user_id;
CREATE INDEX idx_sessions_organizer_id ON sessions(organizer_id);

ALTER TABLE event_series DROP CONSTRAINT event_series_owner_user_id_fkey;
ALTER TABLE event_series RENAME COLUMN owner_user_id TO organizer_id;
ALTER TABLE event_series ADD CONSTRAINT event_series_organizer_id_fkey
    FOREIGN KEY (organizer_id) REFERENCES organizers(id);
DROP INDEX idx_event_series_owner_user_id;
CREATE INDEX idx_event_series_organizer_id ON event_series(organizer_id);

ALTER TABLE events ADD COLUMN organizer_id TEXT;
UPDATE events e
SET organizer_id = owner.user_id
FROM event_memberships owner
WHERE owner.event_id = e.id AND owner.role = 'owner';
ALTER TABLE events ALTER COLUMN organizer_id SET NOT NULL;
ALTER TABLE events ADD CONSTRAINT events_organizer_id_fkey
    FOREIGN KEY (organizer_id) REFERENCES organizers(id);
CREATE INDEX idx_events_organizer_id ON events(organizer_id);
