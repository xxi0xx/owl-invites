# Gate 2: invitation domain and RSVP security boundary

Status: implementation branch. Migration 34 establishes the invitation
security boundary and performs the one-way legacy RSVP mapping. Migration 35
removes the Gate 1 identity and event-ownership shadows after enforcing parity.

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

## Capability design

`OWL_INVITES_SECRET_KEY` is critical restore material. Losing it invalidates
every reconstructable private and open-enrollment capability even if the
database is restored successfully. It must be backed up with the database and
must not be logged or committed.

Private capability material is reconstructed as a domain-separated HMAC over
an opaque random `access_id` and the invitation's `token_version`. Only those
non-secret selectors are stored. A private URL carries the capability in the
fragment so it is not sent in the initial HTTP request or request logs. The SPA
exchanges it once for a random limited invitation session, stores only the
session hash server-side, and removes the fragment from browser history.

Per-invitation rotation increments `token_version` and revokes existing
invitation sessions. Revocation blocks both capabilities and sessions. Those
operations are separate from global secret rotation: changing the global key
invalidates every capability at once and is an operator-controlled recovery
event, not an ordinary invitation operation.

Recovery capabilities are independent random values, hashed at rest,
short-lived, and atomically single-use. Consuming one creates an invitation
session without rotating the primary capability. The public request response
is identical whether zero, one, or several stored destinations match.

Open links are enrollment capabilities, never household capabilities. There
is at most one per event. Each successful enrollment creates a new isolated
invitation and never searches, claims, merges, or mutates invitations by name,
email, or phone. Open capacity counts allocated guest seats and is distinct
from private invitation allocation. Gate 2 has no waitlist.

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
| Private invitation delivery and RSVP | Adapt to invitation sessions and per-guest responses. |
| Messaging | Adapt organizer delivery to invitation destinations; defer guest-to-organizer threads. |
| Reminders and cancellation notices | Adapt delivery to invitation destinations and invitation links. |
| Notification log | Adapt nullable ownership from attendee to invitation. |
| Comments / guestbook | Disable and defer; do not preserve RSVP-token access. |
| CSV import/export | Replace attendee import/export with invitation/guest semantics. |
| Statistics | Adapt headcount and response totals to guests and guest responses. |
| Event series | Preserve event generation; do not copy household invitations between occurrences. |
| Retention / cleanup | Rely on event cascades for the invitation domain; adapt owner lookup to memberships. |
| Invitation-card rendering | Preserve event card design but expose it only through a valid invitation session or open-enrollment capability. |
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

`attendees`, `attendee_answers`, `plus_ones`, `rsvp_token`, and `share_token`
are removed only after the one-way migration assertions pass and all mounted
routes use invitation authorization. Dormant legacy handlers are not a
supported compatibility API.

## Authorization matrix

| Caller | Event administration | Private invitation data | Recovery | Open enrollment |
| --- | --- | --- | --- | --- |
| Anonymous | none | none | generic request only | valid enabled capability only |
| Unrelated organizer | none | none | generic request only | same as anonymous |
| Event co-host | manage event invitations | organizer view for that event | no recovery disclosure | configure only if explicitly permitted by event management policy |
| Event owner | full event invitation management | organizer view for that event | no recovery disclosure | configure |
| Instance admin without membership | no implicit event access | none | no disclosure | no implicit configuration access |
| Instance admin with membership | rights of explicit membership | organizer view for that event | no disclosure | rights of explicit membership |
| Valid invitation capability | exchange only | its invitation only | not applicable | none |
| Rotated capability | denied | none | not applicable | none |
| Revoked invitation | denied | none | generic recovery response | none |
| Invitation session | its invitation only | its invitation only | not applicable | none |
| Recovery capability | exchange once for its invitation | no direct read | atomic single use | none |
| Open enrollment capability | no event administration | cannot read any existing invitation | none | create a new isolated invitation only |
