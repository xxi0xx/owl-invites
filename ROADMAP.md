# Owl Invites product roadmap

This document is the canonical high-level roadmap for Owl Invites after the
verified Gate 5 product-maturation checkpoint. It describes direction and the
evidence used to choose work; it is not an issue tracker, release schedule, or
promise that every exploratory item will ship.

## 1. Product principles

### Respect

Owl Invites should respect organizers, operators, guests, and their data. It
must not manipulate people into choices that benefit the project at their
expense.

### Trust

Behavior should be understandable and defaults should be unsurprising. Avoid
hidden behavior, unnecessary telemetry, lock-in, and dark patterns. When the
product cannot know or guarantee something—such as whether an accepted email
reached an inbox—it should say so.

### User control

Automation should reduce work without taking control away from the operator.
The preferred sequence for consequential automation is:

```text
detect -> explain -> propose -> confirm -> execute -> verify
```

### Simplicity

Complex infrastructure should not require a complex user experience. Make the
common path simple while preserving an advanced or manual path for operators
who need it.

### Honest communication

Failures, limitations, security behavior, operational requirements, and any
future pricing should be described plainly. Product language must not conceal
tradeoffs or use manipulative urgency.

### Privacy

Collect and expose only what is necessary. Contacts remain delivery and
recovery destinations, not identity. The authorization, household-isolation,
capability, recovery, and open-enrollment boundaries established in Gates 1–5
remain product principles, not temporary implementation details.

### Data portability

Owl Invites should make users' event data reasonably exportable and should not
use data captivity as a retention strategy. Self-hosted and any future managed
offerings should preserve practical ways for operators to retrieve and move
their data.

### Open-source respect

Owl Invites remains MIT-licensed. Self-hosted users should not be intentionally
degraded to force monetization, and the core application should not acquire a
mandatory activation server or cloud dependency merely to enable a business
model. This roadmap does not change the license.

## 2. Current state

The verified Gate 5 baseline is a coherent self-hosted invitation product, but
it is not yet an approved stable release. Today it provides:

- a bootstrap-token-protected first run, persistent users and administrators,
  owner/cohost event memberships, and explicit audited recovery boundaries;
- household Invitations with organizer-assigned and holder-managed additional
  Guests, first-class attendance, scoped questions and answers, and isolated
  RSVP responses;
- domain-separated private, open-enrollment, household-session, and recovery
  capabilities with rotation, revocation, secret fingerprinting, and no
  contact-based identity;
- open enrollment that always creates a new isolated household;
- previewed, server-revalidated, transactional household CSV import using an
  explicit grouping key, plus safe organizer reporting export;
- guest presentation of the configured invitation card, event details, guest
  controls, attendance, questions, and response state;
- organizer household search, filters, safe editing, explicit delivery,
  resend, capability rotation, revocation, and honest delivery visibility;
- one-way targeted messages and bounded in-process reminders;
- verified SQLite bundles and a tested PostgreSQL recovery procedure that
  preserve RSVP state, uploads, secret identity, and existing household
  capabilities;
- tested SQLite and PostgreSQL operation, a hardened production deployment,
  readiness and graceful shutdown, and amd64/arm64 images; and
- one authoritative product gate, immutable SHA-only review artifacts, SBOM,
  BuildKit provenance, and GitHub attestation.

The engineering history and exact boundaries are recorded in
[Gate 1](docs/gate-1-foundation.md),
[Gate 2](docs/gate-2-invitation-domain.md),
[Gate 3](docs/gate-3-production-readiness.md),
[Gate 4](docs/gate-4-owl-rebrand.md), and
[Gate 5](docs/gate-5-product-maturation.md). Operators should also use the
[deployment](docs/operations/production-deployment.md),
[backup and restore](docs/operations/backup-restore.md), and
[release integrity](docs/operations/releases.md) guides.

## 3. Path to v1.0

Gate 5 reaching its engineering bar does not approve a stable release. The
immediate path is:

```text
verified Gate 5
  -> release-candidate review
  -> integrated security review
  -> production dogfooding
  -> defect burn-down
  -> release-readiness review
  -> explicit human decision
  -> v1.0.0
```

The [release-candidate review plan](docs/release-candidate-review.md) defines
the next review. Findings should be classified by release impact, corrected on
normal product branches, and reverified through the authoritative gate. The
actual `v1.0.0` tag, image aliases, release notes, and publication require a
separate explicit human decision.

## 4. Near-term v1.x priorities

Likely near-term work, whether justified during release-candidate burn-down or pursued in early v1.x development, should remain focused:

1. Magic Setup and Configuration Doctor investigations that reduce deployment
   and configuration friction without weakening operator control.
2. Product and operational refinements supported by dogfooding, production
   defects, or repeated user evidence.
3. Focused organizer-workflow, invitation-design, reporting, mobile, and
   accessibility improvements where observed friction is material.
4. Delivery and reliability improvements when operational evidence justifies
   their cost and security model.

These are priority areas, not preapproved releases or fixed scope.

## 5. Magic Setup

Magic Setup is a major candidate for early v1.x development.

> Owl Invites should feel like it understands the operator's environment, asks
> only what it cannot safely determine, explains what it observes, automates
> tedious work when invited, immediately verifies configuration, and always
> leaves the operator in control.

Its design principles are:

- detect before asking;
- prefill before requiring input;
- test immediately;
- explain failures in human language;
- automate when invited;
- never hide consequential actions; and
- always provide a manual or advanced path.

“Magic” must never mean opaque. Investigation may cover environment and
deployment discovery; public URL detection; reverse-proxy and trusted-proxy
diagnostics; DNS and TLS validation; Cloudflare detection and guided setup;
least-privilege Cloudflare API token guidance; optional API-assisted DNS record
creation; email-provider selection and guided Resend, Postmark, SES, or generic
SMTP setup; sending-domain and mail-DNS verification; a test-email flow;
secret and recovery-material validation; persistent-storage checks; backup
configuration; and production-readiness diagnostics.

Each area is a roadmap candidate. Provider support and automation should be
selected only after threat modeling, operational research, and user evidence;
none is an implementation commitment here.

## 6. Configuration Doctor

Configuration Doctor is the reusable diagnostics counterpart to Magic Setup,
not a separate subsystem:

```text
Magic Setup          uses shared diagnostics during onboarding
Configuration Doctor uses the same diagnostics throughout installation life
```

A future health overview could assess public URL, DNS, TLS, reverse proxy,
email, database, persistent storage, secret recovery, and backups. Results
should distinguish observation from inference and suggest a safe next action.

```text
Public URL          ✓
DNS                 ✓
TLS                 ✓
Reverse proxy       ✓
Email               ✓
Database            ✓
Persistent storage  ✓
Secret recovery     ✓
Backups             ✓
```

Prefer human explanations such as:

> The SMTP server accepted the connection but rejected the credentials. The
> host and port appear reachable.

over `SMTP error 535`, and:

> Guests are reaching Owl Invites at X, but the configured public URL is Y.

over `BASE_URL mismatch`.

Diagnostics must not disclose secrets or imply certainty they do not have.
This section records direction only; no diagnostic system is implemented by
this roadmap.

## 7. Reliability and operational evolution

The current operational foundation—readiness, graceful shutdown, immutable
identified images, dual architectures, recovery drills, capability-key
fingerprints, and supply-chain evidence—should be preserved.

Near-term evolution should improve operator understanding of configuration,
backup freshness, restore readiness, delivery failures, and upgrade state.
Reliability architecture should respond to evidence. For example, current
notification retries and reminder execution are bounded and in-process; a
durable queue or outbox should be considered only if real failure patterns
justify its operational and capability-security complexity.

SQLite's database snapshot and subsequent upload copy are not one atomic
cross-store snapshot, PostgreSQL recovery remains operator-orchestrated, and
provider acceptance is not proof of inbox delivery. Future UX and diagnostics
should make those limits easier to operate, not hide them.

### Automation safety

Before Owl Invites changes an external service, it should communicate:

- which service and resource would change;
- the exact proposed change and why it is required;
- the permissions required; and
- whether and how the change can be reversed.

Prefer scoped, least-privilege API tokens over broad account credentials.
Manual instructions must remain available. Provider credentials should not be
retained longer than necessary unless a clear product reason, storage boundary,
and deletion path have been reviewed.

## 8. Commercial possibilities

Monetization is explicitly undecided. No billing, paid plan, entitlement,
activation, licensing, or commercial product is announced by this roadmap.

### Community

The model under consideration keeps self-hosted Owl Invites free with the full
core application, no mandatory activation, and no artificial feature
restrictions.

### Supporter

An optional one-time Supporter purchase could voluntarily fund development. It
would not be legal permission required to run MIT software. If a Supporter
designation or certificate is ever implemented, offline verification is
preferred to mandatory phone-home activation.

Possible role language includes Community User, Supporter, Contributor, and
Maintainer. A financial supporter should not be called a Contributor merely
because they paid.

### Owl Invites Cloud

A future paid managed service could charge for genuine service value: hosting,
maintenance, updates, backups, email infrastructure, security operations,
domains and TLS, monitoring, and support. The governing idea is to monetize
convenience and service, not artificial degradation of self-hosting.

If monetization is approved, the preferred architecture separates concerns:

```text
Owl Invites Core (MIT)
  events, invitations, RSVP, self-hosting
  no mandatory billing or activation
              ^ deployed by
Owl Invites Cloud
  accounts, billing, provisioning, lifecycle
  domains, backups, monitoring, usage limits
```

Cloud billing and control-plane concerns should preferably stay outside the
core self-hosted application. A recurring professional or planner offering is
also an exploratory possibility. No pricing or commercial launch is committed.

## 9. Longer-term exploratory areas

Demand and operational evidence may justify investigation of:

- SMS household invitation delivery and recovery with a reviewed security and
  abuse model;
- richer invitation design without becoming a general website builder;
- more advanced, still privacy-respecting reporting;
- professional and planner workflows;
- durable notification/outbox infrastructure if observed reliability needs
  warrant it;
- guest/organizer communication with explicit privacy and moderation
  boundaries; and
- other event-management capabilities repeatedly requested in real use.

Exploratory roadmap items are not promises.

## 10. Explicit non-goals and deferred work

The roadmap does not automatically include AI features, a generalized CRM or
form builder, social networking, ticketing or payments, seating charts, a
wedding website builder, registries, marketing automation, waitlists, photo
galleries, QR check-in, advanced RBAC, branching scripts/calculations, or a
general analytics platform. These should not be added because they are merely
technically possible; future evidence would need to justify a deliberate
product decision.

Contact-based household identity or automatic merging remains a non-goal.
Contacts continue to be delivery and recovery destinations. Self-hosting must
not be intentionally impaired to create a paid upgrade path.

## 11. How roadmap priorities are decided

Priority should be driven by evidence, roughly in this order:

1. security and reliability requirements;
2. production defects;
3. production dogfooding observations;
4. repeated user requests;
5. operator friction;
6. measurable usability and accessibility problems; and
7. strategic product value.

Work should preserve Gates 1–5 invariants, state its user and operational
benefit, and identify its security and recovery consequences. Owl Invites
should avoid feature accumulation simply because something can be built.
