# Project provenance

Owl Invites derives from [OpenRSVP](https://github.com/yannkr/openrsvp), which
was distributed under the MIT License. The known fork baseline is OpenRSVP
1.8.1 at commit `82b8b34ce42a8d0266a8aef56bd8d071bd5df542`.

The upstream notice—`Copyright (c) 2025 OpenRSVP Contributors`—and the complete
MIT terms remain in the repository [LICENSE](../LICENSE). [NOTICE](../NOTICE)
also identifies the upstream source and baseline. The visible Owl Invites name
does not erase, replace, or claim authorship of upstream work.

Redevelopment is staged:

- Gate 1 established the user and event-membership foundation;
- Gate 2 replaced the legacy attendee-token model with isolated household
  invitations, guests, responses, questions, open enrollment, and recovery;
- Gate 3 established production operations, backup/restore, multi-architecture
  images, SBOM, provenance, and attestation; and
- Gate 4 changes project identity while preserving data, capabilities, wire
  compatibility, and upstream attribution.

The full pre-Gate-4 README snapshot and inherited release notes are preserved
in [upstream history](upstream/openrsvp-history.md). The inherited
[implementation plan](upstream/openrsvp-implementation-plan.md) is archived
separately. Those documents retain original names and claims because rewriting
them as Owl Invites history would be inaccurate.

Compatibility references that remain in active code are documented in
[Gate 4](gate-4-owl-rebrand.md) and enforced by the brand-residue lint.
