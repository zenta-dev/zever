# Governance

How zever is run: who decides, how changes land, and where policy lives.
Docs-only; the workflow itself is unchanged.

## Roles

- **Contributors:** anyone opening a PR or issue. Follow
  [CONTRIBUTING](.github/CONTRIBUTING.md) (Conventional Commits,
  `Co-Authored-By` trailer, `CHANGELOG.md` entry under `## [Unreleased]`
  before review).
- **Maintainers:** the `@zenta-dev` org team (see [MAINTAINERS](MAINTAINERS)).
  Maintainers approve PRs, cut releases, and triage security reports.

## Decisions

- Routine changes land with one maintainer approval and green CI.
- Public API changes (new exports, `config` struct/default changes,
  serialization changes) need explicit maintainer review — call it out in
  the PR. Pre-1.0 `0.x` may break with release-note docs; see
  [STABILITY.md](STABILITY.md) for Core vs Extended tiers.
- Disagreements resolve by maintainer decision; contested calls are
  recorded in the PR or issue thread, not reversed silently.

## Releases

Lockstep: one version, per-module tags `<path>/vX.Y.Z` via
`tools/tag-release.sh`. Full pin list in the
[release checklist](.github/CONTRIBUTING.md#release-checklist-lockstep-per-module-tags).
What ships when is tracked in [ROADMAP.md](ROADMAP.md); what broke when is
in [CHANGELOG.md](CHANGELOG.md).

## Security

Report privately via GitHub Security Advisories; fixes ship in the latest
release only (no backports to older lines). Details and response times in
[.github/SECURITY.md](.github/SECURITY.md).

## Policy index

| Topic | Lives in |
|---|---|
| Stability tiers, 1.0 criteria, Go floor, deprecation window | [STABILITY.md](STABILITY.md) |
| 1.0 roadmap checklist | [ROADMAP.md](ROADMAP.md) |
| Contributing, versioning, API compatibility, releases | [.github/CONTRIBUTING.md](.github/CONTRIBUTING.md) |
| Security, supported versions | [.github/SECURITY.md](.github/SECURITY.md) |
| Conduct | [.github/CODE_OF_CONDUCT.md](.github/CODE_OF_CONDUCT.md) |
