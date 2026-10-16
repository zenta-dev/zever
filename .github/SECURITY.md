# Security Policy

## Reporting a Vulnerability

**Do not report security vulnerabilities through public GitHub issues.** Public issues are visible to everyone, including the people who might exploit the vulnerability.

Please report vulnerabilities privately using the ["Report a vulnerability"](https://github.com/zenta-dev/zever/security/advisories/new) page in GitHub Security Advisories instead.

### What to include in a report

To help triage the issue quickly, include:

- The affected version(s) of zever and the Go version used
- A description of the vulnerability and its impact
- A minimal reproduction, including configuration and environment details where relevant
- Steps to trigger the issue
- Where applicable, a proposed fix or mitigation

You may include a patch or proof of concept. Do not include secrets, credentials, or private keys.

Do not open a public pull request that fixes (or describes) an unpatched
vulnerability. Public PRs, issues, and discussion threads are visible to
everyone before a fix ships; use the private advisory channel above so the
fix and the disclosure ship together.

### Scope

All modules in this repository are in scope, with priority on the Core
batteries (`config`, `container`, `db`, `orm`, `router`, `cache`, `queue`,
`session`, `auth`, `crypto`, `password`, `secrets`, `dsl` — see
`../docs/security/threat-model-<battery>.md` for each battery's guarantees and
caller duties) and the security-sensitive adapters (`auth/jwt`, `auth/oidc`,
`crypto/local`, `crypto/kms`, `secrets/vault`, `session/*`, `db/*`). Test
fixtures and deterministic dev-only secrets (e.g. the public dev crypto key)
are out of scope unless they can affect a non-dev deployment.

### What to expect

- An initial acknowledgement within 3 business days of submission.
- A triage assessment and plan within 2 weeks.
- Coordination on a fix and a release date before public disclosure.
- Credit for responsibly reported vulnerabilities, if you want it, in the security advisory.

## Supported Versions

As a pre-1.0 project, security fixes are backported to the latest release. If you rely on a specific release, upgrade to its latest patch version.

| Version | Supported          |
| ------- | ------------------ |
| latest  | :white_check_mark: |
| older   | :x:                |

## Supply Chain

Dependencies are scanned with `govulncheck` on every PR and on a daily
schedule (`.github/workflows/ci.yml` `vulnerability` job,
`.github/workflows/vulncheck.yml`). Reachable vulnerabilities fail the gate;
accepted risk is recorded in `.github/govulncheck-allowlist.json` with a
reason and an expiry date, and expired entries fail closed so every
acceptance is re-triaged. Import-only (unreachable) findings are reported
but never block.

Every CI build publishes a CycloneDX SBOM per module and attests the SBOMs
and release binaries with SLSA build provenance
(`actions/attest-build-provenance`). Verify a release artifact before
running it:

```sh
gh attestation verify dist/zever-linux-amd64 --repo zenta-dev/zever
gh attestation verify sbom/cmd_zever.json --repo zenta-dev/zever
```

The installer (`install.sh`, `install.ps1`) verifies the SHA-256 checksum
before installing and runs `gh attestation verify` automatically when `gh`
is present; without checksum tooling it aborts unless explicitly told not
to (`--insecure-skip-verify` / `ZEVER_INSTALL_INSECURE=1`).

## Responsible Disclosure

We ask that you:

- Give maintainers reasonable time to fix and release before disclosing publicly.
- Not exploit, or help others exploit, the vulnerability beyond what is needed to demonstrate it.

We will coordinate disclosure and credit options with you. If a security issue is disclosed without coordination, we will still fix it, but coordinated reporting is strongly preferred and appreciated.