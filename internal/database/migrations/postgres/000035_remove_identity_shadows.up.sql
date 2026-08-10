DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM organizers o
        LEFT JOIN users u ON u.id = o.id
        WHERE u.id IS NULL
           OR u.email <> o.email
           OR u.normalized_email <> lower(trim(o.email))
           OR u.display_name <> o.name
           OR u.timezone <> o.timezone
           OR u.instance_role <> CASE WHEN o.is_admin THEN 'admin' ELSE 'user' END
    ) THEN
        RAISE EXCEPTION 'organizers/users parity check failed';
    END IF;

    IF EXISTS (
        SELECT e.id
        FROM events e
        LEFT JOIN event_memberships m
          ON m.event_id = e.id AND m.role = 'owner'
        GROUP BY e.id, e.organizer_id
        HAVING COUNT(m.id) <> 1 OR MAX(m.user_id) <> e.organizer_id
    ) THEN
        RAISE EXCEPTION 'events/event_memberships owner parity check failed';
    END IF;
END $$;

ALTER TABLE magic_links DROP CONSTRAINT magic_links_organizer_id_fkey;
ALTER TABLE magic_links RENAME COLUMN organizer_id TO user_id;
ALTER TABLE magic_links ADD CONSTRAINT magic_links_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id);
DROP INDEX idx_magic_links_organizer_id;
CREATE INDEX idx_magic_links_user_id ON magic_links(user_id);

ALTER TABLE sessions DROP CONSTRAINT sessions_organizer_id_fkey;
ALTER TABLE sessions RENAME COLUMN organizer_id TO user_id;
ALTER TABLE sessions ADD CONSTRAINT sessions_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id);
DROP INDEX idx_sessions_organizer_id;
CREATE INDEX idx_sessions_user_id ON sessions(user_id);

ALTER TABLE event_series DROP CONSTRAINT event_series_organizer_id_fkey;
ALTER TABLE event_series RENAME COLUMN organizer_id TO owner_user_id;
ALTER TABLE event_series ADD CONSTRAINT event_series_owner_user_id_fkey
    FOREIGN KEY (owner_user_id) REFERENCES users(id);
DROP INDEX idx_event_series_organizer_id;
CREATE INDEX idx_event_series_owner_user_id ON event_series(owner_user_id);

DROP INDEX idx_events_organizer_id;
ALTER TABLE events DROP COLUMN organizer_id;

DROP TABLE organizers;
