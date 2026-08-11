# Owl Invites release-candidate review plan

This document is the checklist for a future integrated review of the verified
post-Gate-5 product. It does not record a completed review, approve `v1.0.0`,
or authorize publication. The reviewer must choose and record one exact source
commit and immutable image digest before beginning; evidence from another SHA
does not substitute for it.

The review should be skeptical and production-oriented. A real flaw may require
a focused correction to a Gate 1–5 decision, but the review is not permission
to redesign accepted architecture merely for preference.

## Review record

Record these facts with the review evidence:

- exact source commit, image digest, OCI revision, and build identity;
- authoritative GitHub Actions run and result;
- deployment topology, database engine/version, proxy, DNS/TLS, and SMTP
  provider;
- browser and viewport versions used for UX checks;
- backup, restore, and dogfooding dates; and
- every finding, its classification, owner, disposition, and verification
  evidence.

Do not place credentials, raw capabilities, contact lists, or recovery keys in
the review record.

## Product workflow

- [ ] Complete fresh first-run setup with a high-entropy bootstrap token and
      create the first organizer.
- [ ] Log out and back in through the supported account flow.
- [ ] Create, edit, publish, and inspect an event.
- [ ] Configure invitation- and guest-scoped questions, including required
      behavior.
- [ ] Configure the invitation card and validated imagery.
- [ ] Preview and commit a realistic explicit-household CSV import; verify
      equal contacts do not merge households and import sends no email.
- [ ] Create and safely edit a household manually.
- [ ] Explicitly deliver representative invitations and inspect honest
      delivery outcomes, including a controlled failure and retry.
- [ ] Open a delivered household capability in a fresh guest context and
      verify only that household is visible.
- [ ] Confirm the invitation design, event details, assigned guests, attendance,
      additional-guest allowance, and scoped questions form one coherent flow.
- [ ] Submit and later update an RSVP with assigned and allowed additional
      guests.
- [ ] Verify organizer household details, search, response/attendance filters,
      headcount, and answer reporting.
- [ ] Send a targeted one-way message and exercise reminder create, edit,
      cancel, and run behavior without overstating delivery certainty.
- [ ] Export reporting CSV and verify deterministic question columns, Unicode,
      quoting, response state, and formula defanging.
- [ ] Recover private and open-origin households only through stored email
      destinations; verify same-contact households remain isolated.
- [ ] Exercise rotation and revocation confirmations and verify their stated
      capability/session consequences.

## Real deployment

- [ ] Deploy the exact candidate digest with the production Compose definition,
      not a floating tag.
- [ ] Use a real reverse proxy on the intended private network and restrict
      `TRUSTED_PROXIES` to its actual address range.
- [ ] Configure real public DNS and TLS; compare the externally observed URL
      with `BASE_URL`.
- [ ] Configure a real SMTP provider and test authentication, sender/domain
      policy, invitation delivery, recovery, bounce/failure behavior available
      from the supported integration, and human-readable operator errors.
- [ ] Verify persistent database/uploads storage, capability-key custody, file
      ownership, and the hardened read-only container controls.
- [ ] Restart the application during ordinary use and verify readiness, session
      behavior, pending reminder limitations, and retained state.
- [ ] Exercise the documented explicit migration/upgrade sequence from the
      supported prior checkpoint using verified backups.
- [ ] Compare `/health`, `owl-invites version --json`, OCI labels, commit, and
      digest; verify readiness changes and graceful shutdown under SIGTERM.
- [ ] Confirm no host port, Docker socket, unexpected writable path, or secret
      appears in the deployed container or build provenance.

## Recovery

- [ ] Create a representative SQLite backup containing an event, card/upload,
      imported and manual households, assigned/additional guests, submitted
      response, scoped answers, delivery state, and a known capability.
- [ ] Copy and independently verify the bundle; test corruption detection.
- [ ] Perform a destructive restore into new database/uploads paths with the
      matching separately protected secret material.
- [ ] Require readiness, restored presentation and export state, upload access,
      unchanged secret fingerprint, and successful exchange of the pre-backup
      household capability.
- [ ] Prove that a mismatched capability key fails safely before SQLite restore
      targets are committed.
- [ ] For PostgreSQL deployments, create and inspect a custom-format dump and
      upload archive, destroy the test database, restore into a clean database,
      and repeat the state/capability checks with matching tool versions.
- [ ] Confirm the operator understands that migration 36 is irreversible and
      rollback across it means restoring the complete pre-upgrade recovery
      point rather than running a down migration.
- [ ] Record the database/upload snapshot timing limitation and prove the
      selected backup procedure provides an acceptable recovery point.

## Integrated security review

Review the complete post-Gate-5 public surface as one system, not as isolated
historical gates:

- [ ] authentication sessions, magic links, account invitations, logout, and
      disabled-user behavior;
- [ ] first-run bootstrap secrecy, one-time completion, rate limiting, and
      concurrent setup behavior;
- [ ] instance-admin versus event-membership authorization, cohost/owner
      boundaries, audited recovery access, and last-admin protections;
- [ ] private household capability exchange, invitation sessions, recovery,
      HMAC domain separation, token versioning, rotation, revocation, browser
      history, referrers, logs, and restore continuity;
- [ ] open-enrollment inspection/enrollment, capacity concurrency, isolation,
      anti-automation controls, and proof it cannot manage an existing
      household;
- [ ] public recovery timing, atomic rate limits, indistinguishable responses,
      and stored-destination-only delivery;
- [ ] CSV parsing, supported headers, encoding, file/row/field limits,
      normalization, preview/commit revalidation, transaction rollback, and
      authorization for import/template/export;
- [ ] CSV export quoting, deterministic columns, formula injection, contact and
      answer exposure, and authorization;
- [ ] upload and image validation, content types, path traversal, symlinks,
      same-origin serving, presentation DTO allowlisting, and restored files;
- [ ] HTML, Svelte, and email rendering for stored/reflected injection, unsafe
      links, capability leakage, and untrusted event/guest/question content;
- [ ] organizer outbound webhooks and any provider delivery webhook code,
      including which routes are actually mounted, authentication/signature
      verification, replay resistance, body limits, SSRF, and secret handling;
- [ ] proxy trust, forwarded client IP/scheme, HTTPS/HSTS behavior, CSRF, CORS,
      request-size limits, strict JSON decoding, health endpoint disclosure,
      and shutdown admission control;
- [ ] admin/account/event/guest boundaries in every OpenAPI route and frontend
      action, including direct-object-reference attempts;
- [ ] logs, errors, metrics if configured, environment/configuration, database
      DSNs, API keys, SMTP credentials, raw capabilities, and capability-key
      fingerprint behavior; and
- [ ] production Compose/container hardening, dependency and vulnerability
      evidence, immutable build identity, SBOM, provenance, attestation, and
      release policy.

Use unauthenticated, ordinary-user, cohost, owner, instance-admin-without-event-
membership, household-session, rotated/revoked capability, and hostile-input
perspectives. Resolve all high-impact findings or explicitly stop the release
recommendation.

## UX and accessibility

- [ ] Define and record the v1.0 browser-support baseline, then complete the
      critical workflows against it. The initial RC target should include
      current stable Chrome/Chromium, Firefox, Edge, and Safari, with the
      guest RSVP flow also exercised on representative iOS Safari and Android
      Chrome where practical. Record any platform that could not be directly
      tested rather than implying support was verified.
- [ ] Complete the guest invitation and RSVP flow at realistic narrow phone
      widths and with long labels, Unicode names, validation errors, and slow
      or failed requests.
- [ ] Review first-run, login, event list/detail/edit, invitation management,
      import, card design, questions, messages, reminders, export, recovery,
      account, and admin empty/loading/error states.
- [ ] Verify destructive actions use clear confirmations and explain rotate,
      revoke, remove, cancel, and restore consequences.
- [ ] Verify page titles, heading order, landmarks, labels, associated errors,
      keyboard navigation, visible focus, dialog focus behavior, icon-button
      names, live status regions, reduced motion, and usable color contrast.
- [ ] Confirm invitation visuals are supplementary to semantic event and form
      content and that email subject/body/link language is understandable.
- [ ] Record horizontal overflow, clipped content, unreachable controls, focus
      traps, browser differences, and assistive-technology blockers as defects.

## Operational dogfooding

- [ ] Run an actual production-style Owl Invites deployment for a defined
      period of normal organizer and guest use.
- [ ] Include restarts, an upgrade rehearsal, scheduled reminders, ordinary
      delivery failures, backup monitoring, and at least one restore drill.
- [ ] Record observed defects, confusing language, repetitive work, missing
      diagnostics, and operator friction with the exact build and conditions.
- [ ] Classify observations before proposing solutions; do not turn every
      observation into a feature request.
- [ ] Recheck accepted fixes in the same workflow and preserve evidence for
      issues intentionally deferred.

## Issue classification

Use only these release-impact classes:

| Class | Meaning |
| --- | --- |
| **BLOCKER** | Security flaw, data corruption or loss, broken core workflow, unsafe upgrade/restore, or release-pipeline failure. No v1.0 recommendation while unresolved. |
| **SHOULD FIX BEFORE v1.0** | Significant usability or reliability defect with reasonable release-candidate scope. Deferral requires an explicit readiness decision and documented rationale. |
| **POST-1.0** | Enhancement or non-critical issue that does not invalidate the candidate's safety or core workflow. |

Severity and exploitability details may accompany a security finding, but the
review does not need a larger project-management taxonomy.

## Exit criteria

The review may recommend that humans consider a separate v1.0 decision only
when:

- [ ] no known release blockers remain;
- [ ] the complete organizer-to-guest product workflow succeeds;
- [ ] the production-style deployment, restart, and upgrade exercise succeeds;
- [ ] SQLite backup verification and destructive restore succeed, and the
      applicable PostgreSQL recovery procedure succeeds;
- [ ] the integrated security review has no unresolved high-impact findings;
- [ ] critical mobile and accessibility paths are usable;
- [ ] product, deployment, recovery, security, and release documentation match
      observed behavior;
- [ ] authoritative CI is green on the exact candidate commit;
- [ ] the stable-release process, source/tag policy, image aliases, rollback
      decision, and verification evidence are understood and ready; and
- [ ] the review record identifies any accepted residual limitations plainly.

A successful review is a recommendation, not publication authority. Creating
`v1.0.0` and publishing a stable artifact remain a separate explicit human
decision.
