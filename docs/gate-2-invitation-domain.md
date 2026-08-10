# Gate 2: invitation domain and RSVP security boundary

Status: implementation branch, not a production release. Migration 34
establishes the invitation security boundary and performs the one-way legacy
RSVP mapping. Migration 35 removes the Gate 1 identity and event-ownership
shadows after enforcing parity. Migration 36 removes the obsolete attendee,
RSVP-token, guestbook, and two-way-message schema after preserving migrated
invitation children and notification ownership.

## Domain invariants

- An event owns invitations; an invitation is the household and capability
  boundary.
- Contact email, contact phone, and display names are delivery data, not
  identity. No uniqueness or merge rule is based on contact data.
- Every guest belongs to exactly one invitation. Assigned guests cannot be
  deleted or transferred. Additional guests can be created, renamed, and
  removed only within their invitation's allowance.
- Unused allowance is not represented by placeholder guest rows.
- Attendance is `pending`, `attending`, `maybe`, or `declined` and belongs to a
  guest response. It is not a custom question.
- Questions are either invitation-scoped or guest-scoped. Their answers are
  stored in separate ownership tables. Required guest questions apply only to
  attending guests.
- A response is updated with optimistic version checks. Allowance and open
  capacity checks are performed while holding the relevant database row lock.
- Headcount is the number of active guest rows, whether assigned or additional.
  An invitation is a household boundary, not a person count. Open-enrollment
  capacity counts only seats allocated by that open-enrollment configuration;
  private household allocation is deliberately independent.

## Capability design

`OWL_INVITES_SECRET_KEY` is critical restore material. Losing it invalidates
every reconstructable private and open-enrollment capability even if the
database is restored successfully. It must be backed up with the database and
must not be logged or committed.

Household capability material is reconstructed as a domain-separated HMAC over
an opaque random `access_id` and the invitation's `token_version`. Only those
non-secret selectors are stored. `Invitation.source` records allocation
provenance (`private` or `open`); it does not alter the resulting household's
management rights. A household URL carries the capability in the
fragment so it is not sent in the initial HTTP request or request logs. The SPA
exchanges it once for a random limited invitation session, stores only the
session hash server-side, and removes the fragment from browser history.

Household capabilities are durable by design and carry no independent expiry.
Replaying one creates a distinct, time-limited session scoped to the same
invitation; it cannot select or expand into another household. Organizer
rotation or revocation is the expiry mechanism for the durable link. Browser
sessions do expire and are rejected after their configured expiry.

Per-invitation rotation increments `token_version` and revokes existing
invitation sessions. Revocation blocks both capabilities and sessions. Those
operations are separate from global secret rotation: changing the global key
invalidates every capability at once and is an operator-controlled recovery
event, not an ordinary invitation operation.

Recovery capabilities are independent random values, hashed at rest,
short-lived, and atomically single-use. Consuming one creates an invitation
session without rotating the primary capability. The public request response
is identical whether zero, one, or several stored destinations match. Lookup,
token creation, and stored-destination delivery run outside the public response
path, which has a fixed timing floor. Source, destination, and event budgets
are counted and recorded under one database-serialized decision on both SQLite
and PostgreSQL, so parallel transactions cannot race past the configured
limits.

All capability and household responses use `Cache-Control: no-store` and the
application sends `Referrer-Policy: no-referrer`. Capability-bearing URLs use
fragments, so the capability is not sent in the initial HTTP request. Handler
logs exclude raw capabilities and recovery contacts. Invitation cookies are
HTTP-only and SameSite Strict; invitation response mutation uses a CSRF token
cryptographically bound to that invitation session.

Open links are enrollment capabilities, never household capabilities. There
is at most one per event. Each successful enrollment creates a new isolated
invitation and never searches, claims, merges, or mutates invitations by name,
email, or phone. The new invitation receives its own normal household
`access_id`/`token_version` capability in the household HMAC domain, and that
durable management link is delivered to its stored email destination. Losing
the enrollment browser session therefore does not make the household
unrecoverable. Open enrollment requires a valid email destination and always
uses the `email` delivery method; phone-only and SMS enrollment requests are
rejected before persistence. Open capacity counts allocated guest seats and is
distinct from private invitation allocation. Gate 2 has no waitlist.

Resource persistence and delivery are separate outcomes. Once a private
invitation or open-enrollment transaction commits, creation is successful even
if the subsequent SMTP attempt fails. The response carries a capability-free,
nonfatal delivery status/warning; the server logs the failure and the inherited
notification subsystem records provider failures. Open enrollment still sets
the committed invitation-session cookie, so the new household remains visible
and manageable without a duplicate enrollment retry. Private `send=true`
requests are validated for an email method and destination before creation;
organizers can retry a failed send through the explicit delivery endpoint. A
queue/outbox is deferred beyond Gate 2.

## Legacy migration mapping

The one-way migration maps one legacy attendee to one private invitation and
one assigned guest. RSVP attendance and question answers are recoverable.
Legacy `plus_ones` becomes additional-guest allowance because names for those
people were never stored; no placeholder guests are created. `waitlisted`
maps to `pending`. Legacy RSVP/share tokens are never accepted as Owl Invites
capabilities. The legacy random RSVP token is used only as a non-secret access
selector during data migration and still requires a new HMAC proof.

The following information cannot be reconstructed faithfully: names and
answers for numeric plus-ones, whether an unused numeric allowance represented
a real person, and any identity inference previously made from matching
contact fields.

## Dependent feature disposition

| Feature | Gate 2 disposition |
| --- | --- |
| Private invitation delivery and RSVP | Adapt to invitation sessions and per-guest responses. Delivery is email-only and has a result separate from committed creation. |
| Messaging | Adapt organizer delivery to invitation destinations; defer guest-to-organizer threads. |
| Reminders and cancellation notices | Adapt delivery to invitation destinations and invitation links. |
| Notification log | Adapt nullable ownership from attendee to invitation. |
| Comments / guestbook | Disable and defer; do not preserve RSVP-token access. |
| CSV import/export | Disable legacy attendee import/export; defer a capability-safe invitation/guest importer. |
| Statistics | Adapt headcount and response totals to guests and guest responses. |
| Webhooks | Retain `event.published` and `event.cancelled`; legacy RSVP/comment subscriptions are inert and rejected by Gate 2 create/update validation. |
| Event series | Preserve event generation; do not copy household invitations between occurrences. |
| Retention / cleanup | Rely on event cascades for the invitation domain; adapt owner lookup to memberships. |
| Invitation-card rendering | Retain organizer customization, but defer guest payload integration; never expose it through a legacy public share token. |
| Waitlist | Delete from active RSVP behavior; no Gate 2 replacement. |
| Legacy public share/contact upsert | Delete. |

## Compatibility-shadow removal criteria

Migration 35 removes `organizers` and `events.organizer_id`. Authentication,
profile management, ownership transfer, event series, notifications, exports,
retention, administration, statistics, and tests now use `users` and
`event_memberships`; all dual-writes have stopped. The migration aborts before
mutation unless organizer/user identity parity and exactly-one-owner event
parity hold. Its pre-upgrade test runs unchanged on SQLite and PostgreSQL and
also verifies preservation of authentication rows, series links, and migrated
invitation children.

Migration 36 removes `attendees`, `attendee_answers`, `event_comments`, and
legacy `messages`; the event `share_token`, contact requirement, capacity,
waitlist, and comments columns; and the series contact/capacity columns. It
also replaces `notification_log.attendee_id` with invitation ownership and
adds `invitation_messages`. The SQLite migration temporarily copies and
restores every Gate 2 child table affected by table rebuilding. The migration
test starts from the legacy schema, runs unchanged on SQLite and PostgreSQL,
and asserts the recovered guest response, question answer, notification owner,
authentication/session rows, series link, and absence of every removed table
and column.

Migration 36 is an explicit one-way cutover. Its destructive deletes cannot be
reversed from the Gate 2 schema, so a down migration is unsupported. Both the
application migrator and the dialect-specific down scripts fail before any
schema mutation if a target below version 36 is requested. Operator rollback
is: restore the verified database and secret material captured before the
upgrade, then run the pre-Gate-2 application against that restored state. A
lossy recreation of empty legacy tables is not a rollback.

The legacy `internal/rsvp`, `internal/message`, and `internal/comment`
implementations and their notification templates are deleted, not merely
unmounted. Legacy `/i/:token` and `/r/:token` frontend paths make no API call,
discard the path token from browser history, and direct the guest to recovery.

## Authorization matrix

| Caller | Event administration | Private invitation data | Recovery | Open enrollment |
| --- | --- | --- | --- | --- |
| Anonymous | none | none | generic request only | valid enabled capability only |
| Unrelated organizer | none | none | generic request only | same as anonymous |
| Event co-host | manage event invitations | organizer view for that event | no recovery disclosure | configure through explicit event membership |
| Event owner | full event invitation management | organizer view for that event | no recovery disclosure | configure |
| Instance admin without membership | no implicit event access | none | no disclosure | no implicit configuration access |
| Instance admin with membership | rights of explicit membership | organizer view for that event | no disclosure | rights of explicit membership |
| Valid invitation capability | exchange only | its invitation only | not applicable | none |
| Rotated capability | denied | none | not applicable | none |
| Revoked invitation | denied | none | generic recovery response | none |
| Invitation session | its invitation only | its invitation only | not applicable | none |
| Recovery capability | exchange once for its invitation | no direct read | atomic single use | none |
| Open enrollment capability | no event administration | cannot read any existing invitation | none | create a new isolated invitation only |

## Error and concurrency behavior

- Malformed, tampered, expired-session, rotated, revoked, and wrong-domain
  capabilities return generic capability failures without disclosing which
  selector or invitation exists.
- Recovery request responses are byte-for-byte equivalent for matching and
  non-matching contacts and use the same public timing floor; deliberately slow
  SMTP delivery does not extend the matching response. Rate-limit fingerprints
  are keyed HMAC values; raw contacts and client identities are not persisted
  in the limiter table. Delivery always uses the invitation's stored
  destination, never caller-supplied contact text.
- A stale response version receives `409 version_conflict`. Concurrent writes
  for one version admit exactly one winner.
- Additional guests beyond the invitation allowance receive
  `409 allowance_exceeded`.
- Open enrollment outside its enabled time window is unavailable without
  revealing configuration details. Concurrent enrollment at the last seat
  admits exactly one household and returns `409 capacity_reached` to the other.
- Open enrollment rejects missing-email, phone-only, SMS, and `none` delivery
  requests before capacity is consumed. Private SMS/phone preferences may be
  stored only as metadata for manual handling; Gate 2 invitation delivery,
  recovery, and capability-bearing broadcasts never silently fall back to
  email for an SMS-preferred household.

## Browser acceptance boundary

The Chromium/Mailpit acceptance flow starts from a fresh database and verifies
setup plus organizer magic-link login before exercising Gate 2. It creates an
event with invitation- and guest-scoped questions, delivers a named household
invitation through SMTP, exchanges the fragment capability for a limited
session, removes the raw capability from browser history, records per-guest
attendance and one allowed additional guest, persists both answer scopes, and
revisits the session to update the response. The organizer view must show the
resulting household and headcount.

The same flow then enables a two-seat open enrollment link and submits it with
the exact email already stored on the named invitation. It asserts that a new
open-source invitation is created, the named household remains unchanged, the
private allocation does not consume open capacity, and a management email is
sent for the new household. The enrollment browser is destroyed; a fresh
browser exchanges the emailed household capability, sees only the open-origin
invitation, and updates its response. A later enrollment is rejected when the
two seats are exhausted. Finally, a one-way organizer broadcast must deliver
to both isolated invitations through Mailpit.

## Gate 2 limitations and boundary

Gate 2 does not provide a waitlist, contact-based identity/claiming, CSV
invitation import/export, guestbook/comments, guest-to-organizer message
threads, SMS invitation delivery/recovery, reliable asynchronous delivery
infrastructure, or guest invite-card rendering. These are explicitly disabled
or deferred and must not be restored using legacy RSVP or share tokens. No
Gate 3 schema or domain behavior is part of this branch.
