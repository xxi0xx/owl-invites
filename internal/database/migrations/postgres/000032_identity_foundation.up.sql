CREATE TABLE users (
    id                   TEXT PRIMARY KEY,
    email                TEXT NOT NULL,
    normalized_email     TEXT NOT NULL UNIQUE,
    display_name         TEXT NOT NULL DEFAULT '',
    timezone             TEXT NOT NULL DEFAULT 'UTC',
    instance_role        TEXT NOT NULL DEFAULT 'user' CHECK(instance_role IN ('admin','user')),
    status               TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('invited','active','disabled')),
    invited_by_user_id   TEXT REFERENCES users(id),
    activated_at         TEXT,
    last_login_at        TEXT,
    created_at           TEXT NOT NULL,
    updated_at           TEXT NOT NULL
);

CREATE INDEX idx_users_normalized_email ON users(normalized_email);
CREATE INDEX idx_users_instance_role ON users(instance_role);
CREATE INDEX idx_users_status ON users(status);

-- Preserve legacy organizer identities while moving authentication and
-- administration to the persistent Owl Invites user model. Existing event
-- foreign keys remain on organizers until the event-membership migration.
INSERT INTO users (
    id, email, normalized_email, display_name, timezone, instance_role,
    status, activated_at, created_at, updated_at
)
SELECT
    id,
    email,
    lower(trim(email)),
    name,
    timezone,
    CASE WHEN is_admin THEN 'admin' ELSE 'user' END,
    'active',
    created_at,
    created_at,
    updated_at
FROM organizers;

CREATE TABLE instances (
    id                   TEXT PRIMARY KEY CHECK(id = 'default'),
    name                 TEXT NOT NULL,
    default_timezone     TEXT NOT NULL DEFAULT 'UTC',
    allow_signups        BOOLEAN NOT NULL DEFAULT FALSE,
    support_email        TEXT NOT NULL DEFAULT '',
    setup_completed_at   TEXT,
    created_at           TEXT NOT NULL,
    updated_at           TEXT NOT NULL
);

INSERT INTO instances (
    id, name, default_timezone, allow_signups, support_email,
    setup_completed_at, created_at, updated_at
)
SELECT
    'default',
    COALESCE(NULLIF((SELECT value FROM instance_config WHERE key = 'instance_name'), ''), 'Owl Invites'),
    COALESCE(NULLIF((SELECT value FROM instance_config WHERE key = 'default_timezone'), ''), 'UTC'),
    CASE WHEN (SELECT value FROM instance_config WHERE key = 'allow_signups') = 'true' THEN TRUE ELSE FALSE END,
    COALESCE((SELECT value FROM instance_config WHERE key = 'support_email'), ''),
    CASE
        WHEN EXISTS (SELECT 1 FROM users WHERE instance_role = 'admin' AND status = 'active')
        THEN to_char((CURRENT_TIMESTAMP AT TIME ZONE 'UTC'), 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
        ELSE NULL
    END,
    to_char((CURRENT_TIMESTAMP AT TIME ZONE 'UTC'), 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
    to_char((CURRENT_TIMESTAMP AT TIME ZONE 'UTC'), 'YYYY-MM-DD"T"HH24:MI:SS"Z"');

CREATE TABLE account_invites (
    id                   TEXT PRIMARY KEY,
    target_user_id       TEXT NOT NULL REFERENCES users(id),
    normalized_email     TEXT NOT NULL,
    token_hash           TEXT NOT NULL UNIQUE,
    invited_by_user_id   TEXT NOT NULL REFERENCES users(id),
    event_id             TEXT REFERENCES events(id) ON DELETE CASCADE,
    event_role           TEXT CHECK(event_role IS NULL OR event_role = 'cohost'),
    expires_at           TEXT NOT NULL,
    accepted_at          TEXT,
    revoked_at           TEXT,
    created_at           TEXT NOT NULL
);

CREATE INDEX idx_account_invites_target_user ON account_invites(target_user_id);
CREATE INDEX idx_account_invites_email ON account_invites(normalized_email);
CREATE INDEX idx_account_invites_event ON account_invites(event_id);

CREATE TABLE admin_audit_log (
    id                   TEXT PRIMARY KEY,
    actor_user_id        TEXT REFERENCES users(id) ON DELETE SET NULL,
    actor_kind           TEXT NOT NULL DEFAULT 'user' CHECK(actor_kind IN ('user','cli','system')),
    action               TEXT NOT NULL,
    target_user_id       TEXT REFERENCES users(id) ON DELETE SET NULL,
    event_id             TEXT REFERENCES events(id) ON DELETE SET NULL,
    metadata_json        TEXT NOT NULL DEFAULT '{}',
    created_at           TEXT NOT NULL
);

CREATE INDEX idx_admin_audit_created_at ON admin_audit_log(created_at);
CREATE INDEX idx_admin_audit_target_user ON admin_audit_log(target_user_id);
CREATE INDEX idx_admin_audit_event ON admin_audit_log(event_id);
