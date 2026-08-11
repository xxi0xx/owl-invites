# Gate 4 inherited-identity inventory

Gate 4 starts from main commit
`10f76f0411222cdff246cd57ab303288b09ede86`. The initial case-sensitive
repository scan found 80 `OpenRSVP` and 312 `openrsvp` occurrences. The old Go
module accounted for 222 of the lowercase matches. No `Invia` occurrence was
present.

This inventory classifies the residue before rebranding. Bare `RSVP` is not a
brand residue: it remains conventional domain terminology.

| Class | Initial locations | Gate 4 treatment |
| --- | --- | --- |
| Product brand | Go module/imports, server binary, build scripts, containers, Compose, defaults, UI titles/navigation, email templates/subjects, OpenAPI/package metadata, active README and operator examples | Rename to Owl Invites. |
| Compatibility identifier | `/data/openrsvp.db`, Gate 3 review deployment paths, and explicit operator-provided DSNs | Add controlled legacy discovery and documented offline migration; never rewrite an explicit DSN. |
| Protocol identifier | `X-OpenRSVP-*` webhook headers, `OpenRSVP-Webhook/1.0`, calendar `UID` suffix and `PRODID` | Preserve as isolated legacy protocol constants because changing them can break webhook consumers or duplicate imported calendar entries. |
| Cryptographic identifier | Invitation capability and secret-fingerprint HMAC domains | Already Owl-specific; preserve byte-for-byte so Gate 2 capabilities and Gate 3 restore fingerprints remain valid. |
| Historical/licensing provenance | MIT license, staged-gate history, upstream release notes | Preserve accurately in `LICENSE`, `NOTICE`, provenance documentation, and an explicitly historical upstream changelog. |
| Dead legacy code/assets | `cmd/openrsvp`, shipped `openrsvp` executable, Svelte starter favicon, Satoshi font files | Remove. The canonical server and operator interface become one `owl-invites` executable; replace assets with documented Owl-owned/OFL assets. |
| Domain word | `RSVP`, `RSVPResponse`, RSVP status, RSVP questions | Keep. These terms describe the invitation domain and are not mascot or upstream-product terminology. |

The final `scripts/lint-branding.sh` allowlist must cover only the documented
compatibility, protocol, and provenance cases above. Any remaining inherited
identifier outside that set is a Gate 4 defect.
