# DRAFT (not applied — requires human approval): release signing (cosign) + SBOM attachment

Do **not** modify `.github/workflows/release.yml` without maintainer
approval. This is a proposal for review, referenced from the WS6 security PR.

## Current state (verified in `release.yml`)

- `binaries` builds static binaries per GOOS/GOARCH into `dist/`.
- `sbom` shards CycloneDX SBOM generation (8 shards) into `sbom/*.json`.
- `publish` writes `dist/SHA256SUMS.txt`, attests `dist/*` and `sbom/*.json`
  with SLSA provenance (`actions/attest-build-provenance`, OIDC-minted), and
  attaches everything to the GitHub release.
- Verification today: `gh attestation verify <artifact> --repo zenta-dev/zever`
  plus `sha256sum -c dist/SHA256SUMS.txt`. The installer verifies checksums
  and runs attestation when `gh` is present.

## Proposed change (draft diff sketch)

1. After `Generate SHA256SUMS.txt` in the `publish` job, add a cosign step
   (keyless, OIDC-backed — no long-lived secret to manage):
   - Install: `sigstore/cosign-installer@<pinned-SHA>`.
   - Sign each blob: `cosign sign-blob --yes dist/* sbom/*.json
     --output-signature <file>.sig --output-certificate <file>.pem`
     (keyless mode mints a Fulcio certificate via the job's OIDC identity).
   - Upload the `.sig`/`.pem` files alongside the artifacts in the existing
     `gh release upload/create` invocation.
2. SBOM attachment: already attached (`sbom/*.json` in the release upload).
   The only gap is *per-binary-to-SBOM linkage*: propose also uploading the
   existing `sbom/*.json` CycloneDX files with `cosign attest-blob
   --predicate <sbom> --type cyclonedx` so each binary carries a signed
   statement binding it to its SBOM, verifiable with
   `cosign verify-blob --cert-identity <release-workflow-identity>
   --cert-oidc-issuer https://token.actions.githubusercontent.com`.
3. Document the two new verify commands in `.github/SECURITY.md` (supply
   chain section) and the release notes template.

## Why not just SLSA

SLSA provenance already binds *source → artifact*. Cosign adds *artifact →
named identity* signatures verifiable without `gh` (cosign is a single
static binary, friendlier for air-gapped consumers) and the SBOM-binding
attestation, which SLSA alone does not express per-binary.

## Human-review checklist

- [ ] Adopt keyless vs. HSM-backed keys (suggest keyless first).
- [ ] Confirm artifact glob (`dist/*` includes `SHA256SUMS.txt` — decide
      whether to sign the checksum file itself or the blobs; suggest blobs).
- [ ] Re-pin all action SHAs at adoption time.
- [ ] Decide whether cosign failures fail the release (suggest yes —
      fail-closed like the installer change in #617).
