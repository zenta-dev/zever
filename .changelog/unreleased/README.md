# Changelog fragments (parallel-PR strategy)

Every PR that needs a `CHANGELOG.md` entry writes a **fragment** here instead
of editing `CHANGELOG.md` directly. One file per PR, named
`<stream>-<slug>.md` (for example `v080-config-env-only.md`), so parallel
workstreams never conflict on the same file.

## Format

A fragment holds only list entries, no `##`/`###` headings:

```md
- **Breaking:** `config`: removed `Load` and file discovery; use `FromEnv()`.
- feat(config): stdlib `.env` parser with fail-closed errors.
```

The same rules as `tools/changelog-lint.sh` apply: `- **Breaking:**` only as
an entry prefix, even-space continuation indent, no truncated duplicates.

## Lifecycle

- `make changelog-lint` lints every fragment alongside `CHANGELOG.md`.
- `make apidiff` counts fragment `**Breaking:**` entries as `## [Unreleased]`
  entries, so breaking changes stay covered while unmerged.
- Fragments are compiled into `CHANGELOG.md`'s pending section (and deleted)
  by `make changelog-compile`, run at PR-merge time and by
  `make release-prep`. The pending section is `## [Unreleased]`: the release
  cut renames it to `## [vX.Y.Z] - YYYY-MM-DD` and leaves a fresh, empty
  `## [Unreleased]` section at the top, so the next release always has one
  (a changelog that predates that rule falls back to the top
  `## [vX.Y.Z] - YYYY-MM-DD` section while its version has no git tag yet).
  `make changelog-compile-check` fails when the top section is not
  `## [Unreleased]`. Never edit `CHANGELOG.md` and a fragment for the same
  change: the release notes must carry the entry exactly once. The compile
  step skips (with a warning) an entry that is already in the pending
  section, and `make changelog-lint` fails when an entry appears twice in
  the same section.
