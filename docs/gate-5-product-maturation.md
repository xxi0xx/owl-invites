# Gate 5: product maturation and release candidate

Gate 5 turns the Gate 2 invitation domain into a coherent organizer-to-guest
product workflow. It is a release-candidate engineering gate, not a stable
release declaration. No semantic-version or moving container tag is published
by this gate.

## Scope and preserved boundaries

The product flow now covers event setup, questions and invitation-card design,
household-aware CSV import, explicit delivery, household RSVP, organizer
search/filter/edit operations, one-way messages and reminders, reporting CSV,
and recovery continuity.

The existing domain remains authoritative: an Invitation is a household,
Guests belong to that invitation, and RSVPResponse plus per-guest attendance
and scoped answers record the response. Contacts remain delivery and recovery
destinations, never identity. Import, search, recovery, and open enrollment do
not merge households by email, phone, or name. Open enrollment still always
creates a new isolated Invitation.

Gate 5 does not change private/open/recovery HMAC domains, invitation sessions,
`token_version`, secret fingerprinting, or existing household capabilities. It
adds no SMS household delivery/recovery and no durable queue or outbox.

## Household CSV import

The organizer import uses this exact UTF-8 header:

```csv
household_key,household_label,contact_email,contact_phone,preferred_delivery,additional_guest_allowance,guest_name
smith,Smith Family,smith@example.com,,email,1,Jane Smith
smith,Smith Family,smith@example.com,,email,1,John Smith
garcia,Garcia Family,maria@example.com,,email,0,Maria Garcia
```

Each row is one organizer-assigned guest. `household_key` groups rows only
within that upload; it is neither persisted nor treated as invitation identity.
Rows with the same key must repeat identical household label, contact,
preferred-delivery, and allowance values. Email, phone, and guest names never
group rows. Duplicate guest names and equal contacts across different keys are
valid and remain separate invitations.

The browser first uploads the file to a preview endpoint. Preview returns
normalized households, household/assigned-guest counts, all accumulated
validation errors, and non-blocking warnings such as a contact used by multiple
households. Preview writes nothing. Commit sends the reviewed normalized model
back to the server, where every value and aggregate limit is validated again.
The database commit is one transaction on SQLite and PostgreSQL; any late
failure rolls back the whole import. Commit always creates new private
Invitations and never looks up or changes an existing household.

Import deliberately performs no delivery. The organizer explicitly delivers
households after reviewing the created records.

Validation includes:

- exact supported columns, required columns, blank and duplicate headers;
- malformed CSV, inconsistent row widths, valid UTF-8 (with an optional UTF-8
  BOM), CRLF/LF, quoted commas, and Unicode text;
- required keys, labels, and guest names plus the existing field-length and
  contact normalizers;
- valid email and phone data for the selected delivery method;
- `email`, `sms`, or `none` as manual household metadata, while actual Gate 5
  household delivery remains email-only;
- allowance from 0 through 50 and identical household metadata per key; and
- aggregate file, row, household, and per-household limits.

The limits are 512 KiB, 5,000 data rows/assigned guests, 1,000 households, and
100 assigned guests per household. Keys are limited to 100 characters;
household labels and guest names are limited to 200 characters. The downloadable
template is available in the Invitations import screen and at
`GET /api/v1/events/{eventId}/invitations/import/template`.

## Reporting CSV export

The authenticated event-member export is optimized for organizer reporting,
not round-trip reconciliation. It writes one row per active Guest and repeats
household data. Base columns include stable invitation and guest IDs, household
label and contacts, preferred delivery, source, additional-guest allowance,
guest name and origin, attendance, response state, and first/last submission
times.

Questions become deterministic dynamic columns in event sort order:

- `invitation_answer:<label> [<question-id>]`
- `guest_answer:<label> [<question-id>]`

The stable question ID makes repeated labels unambiguous. Checkbox selections
are rendered with ` | ` separators. UTF-8 values and CSV quoting are preserved.
Every output cell whose first non-space character is `=`, `+`, `-`, or `@` is
prefixed with an apostrophe to prevent spreadsheet formula execution.

Import/template/export and organizer management endpoints require event
membership. Public household sessions cannot call them.

## Guest invitation presentation

The guest household page now renders the organizer's configured invitation
card together with event details, assigned guests, permitted additional guests,
attendance, scoped questions, and submit/update controls. The same presentation
works at a narrow phone viewport and remains supplementary to semantic event
and form content.

An already-authorized household session receives a purpose-built, read-only
presentation DTO containing only the allowlisted template, display text,
validated colors/font, and a safe background image URL. It excludes card IDs,
arbitrary custom data, filesystem paths, organizer controls and membership,
other households, and internal storage information. Same-origin uploads remain
behind the existing upload validation and serving rules. No retired public
event share token was restored.

## Organizer household management

The Invitations screen can list and inspect households, search by household
label, stored contact, or active assigned/additional guest name, and filter by
submitted/not-submitted response plus pending/attending/maybe/declined
attendance. These operations are event-member-only.

Organizers can create households, edit safe contact/label/delivery metadata,
change additional-guest allowance, and rename/add/remove assigned guests.
Assigned-guest removal is soft: response and answer history is not physically
destroyed. Additional guests remain household-holder-managed and are read-only
on the organizer definition form.

Delivery is an explicit, retryable operation. A failed delivery never undoes
the persisted Invitation. Rotate and revoke require confirmations. Rotation
invalidates the old capability and sessions and returns a new credential only
in that explicit operation; revocation disables household access. Raw
capabilities are absent from ordinary list, detail, search, log, and export
surfaces.

The household list shows the latest persisted notification status, provider,
attempt time, and error where present. “Accepted” means the configured provider
accepted the message; it does not claim end-recipient delivery unless provider
tracking records a delivery event.

## Messages, reminders, and delivery limitations

One-way organizer messages target all households or a supported attendance
group. Preview shows the group, subject/body, and eligible household count but
not recipient addresses. The result reports attempted, provider-accepted,
failed, and skipped/suppressed counts. Stored email destinations and the
household's email preference remain authoritative.

Reminder management shows schedule time, target group, current approximate
household count, status, and available failure information. Pending reminders
can be edited or cancelled. Delivery keeps the existing bounded in-process
scheduler and retries; there is no generalized durable scheduler/outbox.
A crash after claim can still require operator inspection or manual action.

SMTP acceptance is not proof of inbox delivery. Provider webhook tracking may
refine sent/delivered/opened/failed states when configured. SMS household
invitation delivery and recovery remain unsupported.

## Mobile and accessibility

The critical guest flow uses responsive card/form spacing, wrapping for long
text, mobile-width controls, labeled attendance controls, semantic fieldsets
for multi-select questions, live status/error regions, keyboard-reachable
controls, visible focus behavior, and reduced decorative motion when the user
requests it. The organizer screens remain desktop-first but avoid obvious
phone-width overflow and retain accessible names for icon-only actions and
confirmed destructive operations.

The release-candidate Playwright flow uses a narrow fresh guest context and
exercises setup, event/question/card configuration, household import and
explicit delivery through Mailpit, household RSVP with an additional guest and
scoped answers, organizer search/status, targeted message and reminder,
reporting export, and same-contact open-enrollment isolation.

## Schema, recovery, and release implications

Gate 5 uses the existing schema and adds no database migration. Migration 36
remains irreversible. SQLite and PostgreSQL still share the same schema version
and migration semantics.

The recovery fixtures now contain a card and upload, contact-equal isolated
private/imported households, assigned and additional guests, RSVP attendance,
invitation- and guest-scoped answers, and persisted delivery state. Restore
verification proves presentation, export, response state, upload, secret
fingerprint, and the pre-backup household capability still work. The verified
bundle metadata remains format version 1 because database snapshots already
carry this state.

Gate 5 may publish only an immutable exact-SHA review image after the complete
authoritative workflow succeeds. A stable release, `latest`, semantic-version
tag, or major/minor alias requires a separate decision.

## Known limitations and deferred work

- Email is the only household delivery and recovery channel.
- Provider acceptance cannot guarantee inbox delivery without provider
  tracking.
- Reminder execution remains bounded and in-process; a crash after claim may
  need manual recovery.
- Import is create-only and has no contact-based reconciliation or “send all”
  action.
- Organizer screens are usable on mobile but remain optimized for desktop.
- No waitlists, seating, public website/page builder, galleries, registries,
  ticketing, payments, QR check-in, advanced RBAC, branching form logic,
  guest chat, AI, CRM, marketing automation, generalized analytics, SMS, or
  generalized durable delivery infrastructure is included.

