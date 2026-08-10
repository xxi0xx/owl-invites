-- Gate 1 compatibility shadows may be removed only when both identity and
-- ownership parity hold. A failed guard aborts the migration before mutation.
CREATE TEMP TABLE identity_shadow_parity_guard (
    ok INTEGER NOT NULL CHECK(ok = 1)
);

INSERT INTO identity_shadow_parity_guard (ok)
SELECT CASE WHEN EXISTS (
    SELECT 1
    FROM organizers o
    LEFT JOIN users u ON u.id = o.id
    WHERE u.id IS NULL
       OR u.email <> o.email
       OR u.normalized_email <> lower(trim(o.email))
       OR u.display_name <> o.name
       OR u.timezone <> o.timezone
       OR u.instance_role <> CASE WHEN o.is_admin = 1 THEN 'admin' ELSE 'user' END
) THEN 0 ELSE 1 END;

INSERT INTO identity_shadow_parity_guard (ok)
SELECT CASE WHEN EXISTS (
    SELECT e.id
    FROM events e
    LEFT JOIN event_memberships m
      ON m.event_id = e.id AND m.role = 'owner'
    GROUP BY e.id, e.organizer_id
    HAVING COUNT(m.id) <> 1 OR MAX(m.user_id) <> e.organizer_id
) THEN 0 ELSE 1 END;

DROP TABLE identity_shadow_parity_guard;

CREATE TABLE magic_links_next (
    id         TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    user_id    TEXT NOT NULL REFERENCES users(id),
    expires_at TEXT NOT NULL,
    used_at    TEXT,
    created_at TEXT NOT NULL
);
INSERT INTO magic_links_next (id, token_hash, user_id, expires_at, used_at, created_at)
SELECT id, token_hash, organizer_id, expires_at, used_at, created_at FROM magic_links;
DROP TABLE magic_links;
ALTER TABLE magic_links_next RENAME TO magic_links;
CREATE INDEX idx_magic_links_token_hash ON magic_links(token_hash);
CREATE INDEX idx_magic_links_user_id ON magic_links(user_id);

CREATE TABLE sessions_next (
    id         TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    user_id    TEXT NOT NULL REFERENCES users(id),
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL
);
INSERT INTO sessions_next (id, token_hash, user_id, expires_at, created_at)
SELECT id, token_hash, organizer_id, expires_at, created_at FROM sessions;
DROP TABLE sessions;
ALTER TABLE sessions_next RENAME TO sessions;
CREATE INDEX idx_sessions_token_hash ON sessions(token_hash);
CREATE INDEX idx_sessions_user_id ON sessions(user_id);

CREATE TEMP TABLE event_series_links AS
SELECT id AS event_id, series_id FROM events WHERE series_id IS NOT NULL;

CREATE TABLE event_series_next (
    id                         TEXT PRIMARY KEY,
    owner_user_id              TEXT NOT NULL REFERENCES users(id),
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
INSERT INTO event_series_next (
    id, owner_user_id, title, description, location, timezone, event_time,
    duration_minutes, recurrence_rule, recurrence_end, max_occurrences,
    series_status, retention_days, contact_requirement, show_headcount,
    show_guest_list, rsvp_deadline_offset_hours, max_capacity, created_at, updated_at
)
SELECT id, organizer_id, title, description, location, timezone, event_time,
       duration_minutes, recurrence_rule, recurrence_end, max_occurrences,
       series_status, retention_days, contact_requirement, show_headcount,
       show_guest_list, rsvp_deadline_offset_hours, max_capacity, created_at, updated_at
FROM event_series;
DROP TABLE event_series;
ALTER TABLE event_series_next RENAME TO event_series;
CREATE INDEX idx_event_series_owner_user_id ON event_series(owner_user_id);
CREATE INDEX idx_event_series_status ON event_series(series_status);
UPDATE events
SET series_id = (SELECT links.series_id FROM event_series_links links WHERE links.event_id = events.id)
WHERE id IN (SELECT event_id FROM event_series_links);
DROP TABLE event_series_links;

DROP INDEX idx_events_organizer_id;
ALTER TABLE events DROP COLUMN organizer_id;

DROP TABLE organizers;
PRAGMA foreign_key_check;
