# Gate 1 foundation review

Gate 1 establishes the identity, authorization, setup, API-contract, and test
foundation for Owl Invites. It intentionally stops before the Gate 2
invitation/household redesign.

## Invariants introduced

1. A fresh instance has no browser-claimable administrator. The first
   administrator can only be created with the environment-only
   `OWL_INVITES_BOOTSTRAP_TOKEN`.
2. Setup is a one-time transaction. Instance settings, the first persistent
   user, administrator role, setup completion timestamp, session, and audit
   entry either commit together or do not commit.
3. Public organizer self-signup defaults off. This does not block
   administrator-issued account invitations, owner-sponsored co-host
   invitations, or acceptance of a capability that was already issued.
4. Instance administration and event-data access are independent. An instance
   administrator receives event access only through an explicit membership or
   an audited recovery action.
5. Every event has exactly one owner membership. Co-host authorization is also
   membership-backed.
6. Magic links and account invitations are single-use capabilities stored only
   as hashes. Consumption and session creation are atomic.
7. Production JSON endpoints reject unknown fields, trailing JSON values, and
   oversized request bodies through the shared strict decoder.
8. Legacy contact-upsert RSVP writes are disabled by default. Unsigned inbound
   SendGrid and SES provider webhooks are not mounted.

## Resulting schema

Migration `000032_identity_foundation` adds:

| Table | Purpose |
| --- | --- |
| `instances` | Singleton setup lock, non-secret instance settings, and `setup_completed_at` |
| `users` | Canonical account identity, status, and instance role |
| `account_invites` | Hashed, expiring, revocable account capabilities and optional sponsored event role |
| `admin_audit_log` | Minimal append-only record of privilege-sensitive actions |

Migration `000033_event_memberships` adds `event_memberships` with `owner` and
`cohost` roles. A partial unique index enforces one owner per event. Existing
event organizers and co-hosts are backfilled, and the old `event_cohosts` table
is removed.

Two compatibility shadows remain deliberately during the staged migration:

- `organizers` is dual-written while older subsystems are moved to `users`.
- `events.organizer_id` mirrors the current owner for legacy reads; membership
  is authoritative for access decisions.

Audit actor and target foreign keys use `ON DELETE SET NULL`, so account
deletion does not erase the fact that a sensitive action occurred.

## Authorization model

| Operation | Instance admin | Event owner | Event co-host | Ordinary user |
| --- | --- | --- | --- | --- |
| Manage instance users/settings | Yes | Only if also admin | Only if also admin | No |
| List or read an event | Only with membership | Yes | Yes | Only with membership |
| Invite an event co-host | Only with membership/ownership | Yes | No | No |
| Transfer ownership normally | No implicit access | Yes | No | No |
| Grant self recovery membership | Audited explicit action | N/A | N/A | No |
| Administrative ownership transfer | Audited explicit action | N/A | N/A | No |

User disablement revokes active sessions. Role and status changes serialize on
the singleton instance row so two concurrent requests cannot remove the last
active administrator.

## Bootstrap and recovery

`GET /api/v1/setup/status` is public and contains no secret. While setup is
required, `POST /api/v1/setup/bootstrap` validates the operator token with a
constant-time comparison and runs under a rate limiter. The raw token is never
stored, logged, or returned. After `setup_completed_at` commits, the endpoint
returns `404` permanently, including if the environment variable remains set.

Ongoing settings use the administrator-only `/api/v1/setup/config` endpoint;
they never reuse bootstrap authorization.

If administrators become unavailable later, a trusted host can run:

```bash
owl-invites admin promote --email operator@example.com
```

The existing user is activated and promoted, and the operation records an
`emergency_role_recovery` audit entry with a CLI actor. The release container
ships this binary alongside the server.

## Account invitations

Administrators can create and revoke account invitations. The API returns
invitation metadata but never the raw capability. Email is the only delivery
path. The link places the raw token in a URL fragment; the browser removes that
fragment from history before exchanging it. Acceptance creates the session and
activates the user atomically.

Owner-sponsored co-host invitations use the same account activation path and
reserve co-host capacity while pending. Acceptance grants the explicit event
membership. `ALLOW_SIGNUPS=false` does not block either invitation path.

## API boundary

The Gate 1 contract is `api/openapi.json`, served at
`GET /api/v1/openapi.json`. Every contracted operation has a unique
`operationId`; every JSON request schema is closed with
`additionalProperties: false`.

`scripts/generate-api-client.mjs` generates
`web/src/lib/api/generated.ts` without network access or third-party generator
state. `npm run check` fails if the generated client differs from the contract.
New setup, authentication, dashboard, user-administration, audit, and event
membership UI flows consume typed operation identifiers.

The contract currently describes the Gate 1 boundary, not every legacy API.
Legacy endpoints remain candidates for deliberate conversion or deletion in
later gates.

## Request and transport security

- The shared JSON decoder limits bodies, rejects unknown keys and trailing
  values, and is used by production JSON handlers.
- Magic-link consumption is transactional and marks a link used only in the
  same transaction that creates the session.
- Forwarded client IP and scheme headers are trusted only from configured
  proxy CIDRs. HSTS and secure-request decisions cannot be spoofed by direct
  clients.
- `Referrer-Policy: no-referrer` limits capability leakage.
- Session mutations remain CSRF-protected; pre-authentication one-time
  capability exchanges have narrow route exemptions and their own rate limits.
- Instance administrators have no implicit RSVP/guest visibility.

## Routing and administration UI

The root route resolves context before showing a destination:

1. Fresh instance -> `/setup`
2. Configured and authenticated -> `/events`
3. Configured and anonymous -> public landing page

The setup UI explains the environment bootstrap token and defaults open signup
off. Administration now exposes Overview, Users & Invites, Audit Log, and
Settings. Account-invite and magic-link pages scrub raw tokens from browser
history before exchange.

## CI and release verification

CI contains independent gates for:

- generated API contract, Svelte type checking, and production frontend build;
- full Go suite with race detection on SQLite;
- full Go suite with race detection against PostgreSQL 16;
- lint and vulnerability scanning;
- a Chromium acceptance test backed by Mailpit SMTP and its supported API;
- the production Docker build after frontend, backend, and E2E gates pass.

The browser acceptance test starts from a fresh database, completes bootstrap,
logs out, requests a magic link, retrieves the delivered email from Mailpit,
opens the real link in Chromium, and verifies the authenticated dashboard.

## Compatibility and Gate 2 boundary

Gate 1 deliberately disables insecure legacy RSVP mutations but does not yet
replace the RSVP domain. It does not introduce private invitation households,
assigned/additional guests, open-enrollment capabilities, recovery
capabilities, invitation/guest answer tables, `OWL_INVITES_SECRET_KEY`, or the
new capability rotation model. Those are Gate 2 decisions and implementation.

The broad OpenRSVP-to-Owl-Invites rebrand is also deferred. Existing binary,
package, database, and UI identifiers remain where changing them would create
unrelated churn. New Gate 1 identifiers use Owl Invites naming.

Before starting Gate 2, review:

- migrations 032 and 033 in both database dialects;
- the OpenAPI contract and generated client diff;
- bootstrap transaction and concurrency tests;
- last-admin, membership, ownership-transfer, and co-host capacity tests;
- the browser/Mailpit acceptance test;
- compatibility shadows and their planned removal point;
- the security assumptions above.
