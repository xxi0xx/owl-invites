CREATE TABLE event_memberships (
    id                   TEXT PRIMARY KEY,
    event_id             TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    user_id              TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role                 TEXT NOT NULL CHECK(role IN ('owner','cohost')),
    granted_by_user_id   TEXT NOT NULL REFERENCES users(id),
    created_at           TEXT NOT NULL,
    updated_at           TEXT NOT NULL,
    UNIQUE(event_id, user_id)
);

CREATE UNIQUE INDEX idx_event_memberships_one_owner
    ON event_memberships(event_id) WHERE role = 'owner';
CREATE INDEX idx_event_memberships_user ON event_memberships(user_id);
CREATE INDEX idx_event_memberships_event_role ON event_memberships(event_id, role);

INSERT INTO event_memberships (
    id, event_id, user_id, role, granted_by_user_id, created_at, updated_at
)
SELECT
    'owner:' || e.id,
    e.id,
    e.organizer_id,
    'owner',
    e.organizer_id,
    e.created_at,
    e.updated_at
FROM events e;

INSERT INTO event_memberships (
    id, event_id, user_id, role, granted_by_user_id, created_at, updated_at
)
SELECT
    c.id,
    c.event_id,
    c.organizer_id,
    'cohost',
    c.added_by,
    c.created_at,
    c.created_at
FROM event_cohosts c
WHERE NOT EXISTS (
    SELECT 1 FROM event_memberships m
    WHERE m.event_id = c.event_id AND m.user_id = c.organizer_id
);

DROP TABLE event_cohosts;
