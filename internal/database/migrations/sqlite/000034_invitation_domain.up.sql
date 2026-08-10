ALTER TABLE event_questions
    ADD COLUMN scope TEXT NOT NULL DEFAULT 'guest'
    CHECK(scope IN ('invitation','guest'));

CREATE TABLE open_enrollments (
    id                   TEXT PRIMARY KEY,
    event_id             TEXT NOT NULL UNIQUE REFERENCES events(id) ON DELETE CASCADE,
    access_id            TEXT NOT NULL UNIQUE,
    token_version        INTEGER NOT NULL DEFAULT 1 CHECK(token_version > 0),
    enabled              INTEGER NOT NULL DEFAULT 0 CHECK(enabled IN (0,1)),
    opens_at             TEXT,
    closes_at            TEXT,
    max_party_size       INTEGER NOT NULL CHECK(max_party_size > 0),
    capacity             INTEGER CHECK(capacity IS NULL OR capacity > 0),
    created_by_user_id   TEXT REFERENCES users(id) ON DELETE SET NULL,
    revoked_at           TEXT,
    created_at           TEXT NOT NULL,
    updated_at           TEXT NOT NULL,
    CHECK(opens_at IS NULL OR closes_at IS NULL OR opens_at < closes_at)
);

CREATE INDEX idx_open_enrollments_event ON open_enrollments(event_id);

CREATE TABLE invitations (
    id                         TEXT PRIMARY KEY,
    event_id                   TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    label                      TEXT NOT NULL,
    contact_email              TEXT,
    normalized_contact_email   TEXT,
    contact_phone              TEXT,
    normalized_contact_phone   TEXT,
    preferred_delivery_method TEXT NOT NULL DEFAULT 'email'
        CHECK(preferred_delivery_method IN ('email','sms','none')),
    additional_guest_allowance INTEGER NOT NULL DEFAULT 0
        CHECK(additional_guest_allowance >= 0),
    source                     TEXT NOT NULL CHECK(source IN ('private','open')),
    open_enrollment_id         TEXT REFERENCES open_enrollments(id) ON DELETE CASCADE,
    access_id                  TEXT NOT NULL UNIQUE,
    token_version              INTEGER NOT NULL DEFAULT 1 CHECK(token_version > 0),
    created_by_user_id         TEXT REFERENCES users(id) ON DELETE SET NULL,
    revoked_at                 TEXT,
    revocation_reason          TEXT,
    created_at                 TEXT NOT NULL,
    updated_at                 TEXT NOT NULL,
    CHECK(
        (source = 'private' AND open_enrollment_id IS NULL) OR
        (source = 'open' AND open_enrollment_id IS NOT NULL)
    )
);

CREATE INDEX idx_invitations_event ON invitations(event_id);
CREATE INDEX idx_invitations_email ON invitations(event_id, normalized_contact_email);
CREATE INDEX idx_invitations_phone ON invitations(event_id, normalized_contact_phone);
CREATE INDEX idx_invitations_open_enrollment ON invitations(open_enrollment_id);

CREATE TABLE guests (
    id            TEXT PRIMARY KEY,
    invitation_id TEXT NOT NULL REFERENCES invitations(id) ON DELETE CASCADE,
    name          TEXT NOT NULL,
    origin        TEXT NOT NULL CHECK(origin IN ('assigned','additional')),
    sort_order    INTEGER NOT NULL DEFAULT 0,
    removed_at    TEXT,
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);

CREATE INDEX idx_guests_invitation ON guests(invitation_id, removed_at, sort_order);

CREATE TABLE rsvp_responses (
    id            TEXT PRIMARY KEY,
    invitation_id TEXT NOT NULL UNIQUE REFERENCES invitations(id) ON DELETE CASCADE,
    version       INTEGER NOT NULL DEFAULT 1 CHECK(version > 0),
    submitted_at  TEXT,
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);

CREATE TABLE guest_responses (
    id               TEXT PRIMARY KEY,
    rsvp_response_id TEXT NOT NULL REFERENCES rsvp_responses(id) ON DELETE CASCADE,
    guest_id         TEXT NOT NULL UNIQUE REFERENCES guests(id) ON DELETE CASCADE,
    attendance       TEXT NOT NULL DEFAULT 'pending'
        CHECK(attendance IN ('pending','attending','maybe','declined')),
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL
);

CREATE INDEX idx_guest_responses_response ON guest_responses(rsvp_response_id);
CREATE INDEX idx_guest_responses_attendance ON guest_responses(attendance);

CREATE TABLE invitation_answers (
    id            TEXT PRIMARY KEY,
    invitation_id TEXT NOT NULL REFERENCES invitations(id) ON DELETE CASCADE,
    question_id   TEXT NOT NULL REFERENCES event_questions(id),
    answer        TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL,
    UNIQUE(invitation_id, question_id)
);

CREATE INDEX idx_invitation_answers_invitation ON invitation_answers(invitation_id);

CREATE TABLE guest_answers (
    id          TEXT PRIMARY KEY,
    guest_id    TEXT NOT NULL REFERENCES guests(id) ON DELETE CASCADE,
    question_id TEXT NOT NULL REFERENCES event_questions(id),
    answer      TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    UNIQUE(guest_id, question_id)
);

CREATE INDEX idx_guest_answers_guest ON guest_answers(guest_id);

CREATE TABLE invitation_sessions (
    id                   TEXT PRIMARY KEY,
    invitation_id        TEXT NOT NULL REFERENCES invitations(id) ON DELETE CASCADE,
    token_hash           TEXT NOT NULL UNIQUE,
    issued_token_version INTEGER NOT NULL CHECK(issued_token_version > 0),
    expires_at           TEXT NOT NULL,
    last_used_at         TEXT,
    revoked_at           TEXT,
    created_at           TEXT NOT NULL
);

CREATE INDEX idx_invitation_sessions_invitation ON invitation_sessions(invitation_id);
CREATE INDEX idx_invitation_sessions_expires ON invitation_sessions(expires_at);

CREATE TABLE invitation_recovery_tokens (
    id             TEXT PRIMARY KEY,
    invitation_id  TEXT NOT NULL REFERENCES invitations(id) ON DELETE CASCADE,
    token_hash     TEXT NOT NULL UNIQUE,
    destination    TEXT NOT NULL CHECK(destination IN ('email','sms')),
    expires_at     TEXT NOT NULL,
    consumed_at    TEXT,
    created_at     TEXT NOT NULL
);

CREATE INDEX idx_invitation_recovery_invitation ON invitation_recovery_tokens(invitation_id);
CREATE INDEX idx_invitation_recovery_expires ON invitation_recovery_tokens(expires_at);

CREATE TABLE invitation_recovery_attempts (
    id                     TEXT PRIMARY KEY,
    event_id               TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    source_fingerprint     TEXT NOT NULL,
    destination_fingerprint TEXT NOT NULL,
    created_at             TEXT NOT NULL
);

CREATE INDEX idx_recovery_attempts_event_time ON invitation_recovery_attempts(event_id, created_at);
CREATE INDEX idx_recovery_attempts_source_time ON invitation_recovery_attempts(source_fingerprint, created_at);
CREATE INDEX idx_recovery_attempts_destination_time ON invitation_recovery_attempts(destination_fingerprint, created_at);

-- One legacy attendee becomes one isolated private invitation and one assigned
-- guest. The legacy random RSVP token is retained only as a non-secret access
-- selector; it is never accepted without a new domain-separated HMAC proof.
INSERT INTO invitations (
    id, event_id, label, contact_email, normalized_contact_email,
    contact_phone, normalized_contact_phone, preferred_delivery_method,
    additional_guest_allowance, source, access_id, token_version,
    created_by_user_id, created_at, updated_at
)
SELECT
    'legacy-invitation:' || a.id,
    a.event_id,
    a.name,
    NULLIF(trim(a.email), ''),
    NULLIF(lower(trim(a.email)), ''),
    NULLIF(trim(a.phone), ''),
    NULLIF(trim(a.phone), ''),
    CASE WHEN a.contact_method IN ('email','sms') THEN a.contact_method ELSE 'none' END,
    CASE WHEN a.plus_ones > 0 THEN a.plus_ones ELSE 0 END,
    'private',
    a.rsvp_token,
    1,
    (SELECT m.user_id FROM event_memberships m
     WHERE m.event_id = a.event_id AND m.role = 'owner' LIMIT 1),
    a.created_at,
    a.updated_at
FROM attendees a;

INSERT INTO guests (
    id, invitation_id, name, origin, sort_order, created_at, updated_at
)
SELECT
    'legacy-guest:' || a.id,
    'legacy-invitation:' || a.id,
    a.name,
    'assigned',
    0,
    a.created_at,
    a.updated_at
FROM attendees a;

INSERT INTO rsvp_responses (
    id, invitation_id, version, submitted_at, created_at, updated_at
)
SELECT
    'legacy-response:' || a.id,
    'legacy-invitation:' || a.id,
    1,
    CASE WHEN a.rsvp_status = 'pending' OR a.rsvp_status = 'waitlisted' THEN NULL ELSE a.updated_at END,
    a.created_at,
    a.updated_at
FROM attendees a;

INSERT INTO guest_responses (
    id, rsvp_response_id, guest_id, attendance, created_at, updated_at
)
SELECT
    'legacy-guest-response:' || a.id,
    'legacy-response:' || a.id,
    'legacy-guest:' || a.id,
    CASE WHEN a.rsvp_status = 'waitlisted' THEN 'pending' ELSE a.rsvp_status END,
    a.created_at,
    a.updated_at
FROM attendees a;

INSERT INTO guest_answers (
    id, guest_id, question_id, answer, created_at, updated_at
)
SELECT
    'legacy-guest-answer:' || aa.id,
    'legacy-guest:' || aa.attendee_id,
    aa.question_id,
    aa.answer,
    aa.created_at,
    aa.updated_at
FROM attendee_answers aa;
