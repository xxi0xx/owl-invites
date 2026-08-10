# Release integrity

> [!IMPORTANT]
> Gate 3 prepares and tests release machinery. It does not publish the first
> stable Owl Invites release. A manually dispatched Gate 3 review may publish
> only `sha-<full-commit>` so the actual registry SBOM/provenance chain can be
> tested; that review artifact is not a production release.

## One verification boundary

`.github/workflows/verify.yml` is a reusable, authoritative product gate. Both
branch/PR CI and release CI call it. It includes frontend/generated contract,
route lint, production frontend build, Go vet, SQLite full/race/migrations,
PostgreSQL 16 full/race/migrations, Chromium/Mailpit flows, golangci-lint,
govulncheck, PostgreSQL disaster recovery, amd64/arm64 container builds, and
the production-container hardening/SIGTERM drill.

Verification jobs have read-only repository permissions. Package write, OIDC,
and attestation permissions exist only in the publication job after the shared
gate succeeds.

## Tag and source policy

Release tags must match exactly `vX.Y.Z`, with no leading zeroes and no implicit
prerelease syntax. The validator rejects `v1`, `v1.2`, `latest`, arbitrary
`v*` strings, and tags whose commit is not contained in fetched
`origin/main`. Policy tests exercise both accepted and rejected histories.

Published aliases are limited to:

```text
ghcr.io/xxi0xx/owl-invites:vX.Y.Z
ghcr.io/xxi0xx/owl-invites:sha-<full-commit>
```

Neither is the deployment authority. CI surfaces the manifest-list
`sha256:<digest>`; deploy:

```text
ghcr.io/xxi0xx/owl-invites@sha256:<digest>
```

No `latest`, major-only, or major/minor aliases are created.

Manual review publication is restricted to `main` or `codex/*` branch refs,
is blocked on the same authoritative product gate, and emits only the full-SHA
alias. It cannot mint a stable version tag. Leaving the dispatch option off
runs the multi-architecture release build without publishing anything.

Before this workflow is present on the default branch, GitHub cannot dispatch
it manually. The bootstrap review path is an exact annotated or lightweight
tag named `review-sha-<full-commit>` on that same commit. The validator rejects
any name/target mismatch; the workflow still emits only the SHA image alias.
This review tag is not a semantic-version release tag and creates no GitHub
Release.

## Identity, SBOM, and provenance

The release tag and full commit are injected into both binaries through
`scripts/build-go.sh`. The same values become OCI source/revision/version
labels. Release builds cover `linux/amd64` and `linux/arm64` from bases pinned
to manifest-list digests.

Docker BuildKit emits an OCI SPDX SBOM and `mode=max` provenance. The narrowly
permissioned publication job also uses `actions/attest` with GitHub OIDC to
attach keyless build provenance to the exact registry digest. No long-lived
signing key is introduced. Because this is a user-owned public repository, the
action pushes the attestation to the OCI registry and deliberately does not
request an organization-only GitHub storage record.

Release CI verifies the SPDX document is present and non-empty and verifies the
GitHub attestation against the repository identity. Independent operator
verification is:

```bash
image='ghcr.io/xxi0xx/owl-invites@sha256:<digest>'
gh attestation verify "oci://$image" --repo xxi0xx/owl-invites
docker buildx imagetools inspect "$image" \
  --format '{{ json (index .SBOM "linux/amd64").SPDX }}' \
  | jq -e '.SPDXID == "SPDXRef-DOCUMENT" and (.packages | length > 0)'
```

The digest, not a human-written release note, binds identity. Build arguments
must never contain secrets because `mode=max` provenance can expose build
parameters. The release workflow passes only version, commit, build state, and
source URL.

Mechanism references:

- [GitHub artifact attestations](https://docs.github.com/actions/security-for-github-actions/using-artifact-attestations/use-artifact-attestations)
- [actions/attest](https://github.com/actions/attest)
- [Docker SBOM attestations](https://docs.docker.com/build/metadata/attestations/sbom/)
- [Docker GitHub Actions attestations](https://docs.docker.com/build/ci/github-actions/attestations/)

Every Action remains pinned to a full commit SHA. Dependabot may update the SHA
and version annotation through review; it must not replace the pin with a
floating tag.
