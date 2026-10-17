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
- Fragments are compiled into `CHANGELOG.md` `## [Unreleased]` (and deleted)
  by `make changelog-compile`, run at PR-merge time and by
  `make release-prep`. Never edit `CHANGELOG.md` and a fragment for the same
  change; the compile step would duplicate the entry.
