CREATE TABLE organizers (
    id         TEXT PRIMARY KEY,
    email      TEXT NOT NULL UNIQUE,
    name       TEXT NOT NULL DEFAULT '',
    timezone   TEXT NOT NULL DEFAULT '',
    is_admin   INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
INSERT INTO organizers (id, email, name, timezone, is_admin, created_at, updated_at)
SELECT id, email, display_name, timezone,
       CASE WHEN instance_role = 'admin' THEN 1 ELSE 0 END,
       created_at, updated_at
FROM users;
CREATE INDEX idx_organizers_email ON organizers(email);

CREATE TABLE magic_links_previous (
    id           TEXT PRIMARY KEY,
    token_hash   TEXT NOT NULL UNIQUE,
    organizer_id TEXT NOT NULL REFERENCES organizers(id),
    expires_at   TEXT NOT NULL,
    used_at      TEXT,
    created_at   TEXT NOT NULL
);
INSERT INTO magic_links_previous (id, token_hash, organizer_id, expires_at, used_at, created_at)
SELECT id, token_hash, user_id, expires_at, used_at, created_at FROM magic_links;
DROP TABLE magic_links;
ALTER TABLE magic_links_previous RENAME TO magic_links;
CREATE INDEX idx_magic_links_token_hash ON magic_links(token_hash);
CREATE INDEX idx_magic_links_organizer_id ON magic_links(organizer_id);

CREATE TABLE sessions_previous (
    id           TEXT PRIMARY KEY,
    token_hash   TEXT NOT NULL UNIQUE,
    organizer_id TEXT NOT NULL REFERENCES organizers(id),
    expires_at   TEXT NOT NULL,
    created_at   TEXT NOT NULL
);
INSERT INTO sessions_previous (id, token_hash, organizer_id, expires_at, created_at)
SELECT id, token_hash, user_id, expires_at, created_at FROM sessions;
DROP TABLE sessions;
ALTER TABLE sessions_previous RENAME TO sessions;
CREATE INDEX idx_sessions_token_hash ON sessions(token_hash);
CREATE INDEX idx_sessions_organizer_id ON sessions(organizer_id);

CREATE TEMP TABLE event_series_links AS
SELECT id AS event_id, series_id FROM events WHERE series_id IS NOT NULL;

CREATE TABLE event_series_previous (
    id                         TEXT PRIMARY KEY,
    organizer_id               TEXT NOT NULL REFERENCES organizers(id),
    title                      TEXT NOT NULL,
    description                TEXT NOT NULL DEFAULT '',
    location                   TEXT NOT NULL DEFAULT '',
    timezone                   TEXT NOT NULL DEFAULT 'America/New_York',
    event_time                 TEXT NOT NULL,
    duration_minutes           INTEGER,
    recurrence_rule            TEXT NOT NULL CHECK(recurrence_rule IN ('weekly','biweekly','monthly')),
    recurrence_end             TEXT,
    max_occurrences            INTEGER,
    series_status              TEXT NOT NULL DEFAULT 'active' CHECK(series_status IN ('active','stopped')),
    retention_days             INTEGER NOT NULL DEFAULT 30,
    contact_requirement        TEXT NOT NULL DEFAULT 'email',
    show_headcount             INTEGER NOT NULL DEFAULT 0,
    show_guest_list            INTEGER NOT NULL DEFAULT 0,
    rsvp_deadline_offset_hours INTEGER,
    max_capacity               INTEGER,
    created_at                 TEXT NOT NULL,
    updated_at                 TEXT NOT NULL
);
INSERT INTO event_series_previous (
    id, organizer_id, title, description, location, timezone, event_time,
    duration_minutes, recurrence_rule, recurrence_end, max_occurrences,
    series_status, retention_days, contact_requirement, show_headcount,
    show_guest_list, rsvp_deadline_offset_hours, max_capacity, created_at, updated_at
)
SELECT id, owner_user_id, title, description, location, timezone, event_time,
       duration_minutes, recurrence_rule, recurrence_end, max_occurrences,
       series_status, retention_days, contact_requirement, show_headcount,
       show_guest_list, rsvp_deadline_offset_hours, max_capacity, created_at, updated_at
FROM event_series;
DROP TABLE event_series;
ALTER TABLE event_series_previous RENAME TO event_series;
CREATE INDEX idx_event_series_organizer_id ON event_series(organizer_id);
CREATE INDEX idx_event_series_status ON event_series(series_status);
UPDATE events
SET series_id = (SELECT links.series_id FROM event_series_links links WHERE links.event_id = events.id)
WHERE id IN (SELECT event_id FROM event_series_links);
DROP TABLE event_series_links;

ALTER TABLE events ADD COLUMN organizer_id TEXT REFERENCES organizers(id);
UPDATE events
SET organizer_id = (
    SELECT owner.user_id FROM event_memberships owner
    WHERE owner.event_id = events.id AND owner.role = 'owner'
);
CREATE INDEX idx_events_organizer_id ON events(organizer_id);

PRAGMA foreign_key_check;
