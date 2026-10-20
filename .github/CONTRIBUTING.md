# Contributing to zever

Thanks for contributing to zever. This guide explains how to set up the project, how to write code that fits the codebase, and how to get a change reviewed and merged. Common tasks are driven by the [Makefile](../Makefile); run `make help` to list them.

## Table of Contents

- [Getting Started](#getting-started)
- [Development Workflow](#development-workflow)
- [Go Coding Standards](#go-coding-standards)
- [Testing Standards](#testing-standards)
- [Benchmarks](#benchmarks)
- [API Compatibility](#api-compatibility)
- [Versioning](#versioning)
- [Commit Convention](#commit-convention)
- [Changelog](#changelog)
- [License](#license)
- [Architecture](../docs/architecture.md)
- [AI-assisted development](../docs/ai-assisted-development.md)
- [Good first issues (draft pool)](../docs/plans/good-first-issues.md)

## Getting Started

### Prerequisites

Required:

- Go 1.27 or newer (this project uses `go 1.27.2` in each module's `go.mod`)

Install the pinned development tools (`golangci-lint`, `govulncheck`, and `cyclonedx-gomod`):

```bash
make setup
```

### Repository Setup

```bash
git clone git@github.com:zenta-dev/zever.git
cd zever
```

### Dependency installation

Dependencies are managed by the Go module system. No separate dependency manager is required:

```bash
make download   # go mod download
```

### Running tests

```bash
make test        # go test ./...
make test-race   # go test -race ./...
```

### Formatting

```bash
make fmt      # check formatting, fails on unformatted files
make fmt-fix  # format all files in place
```

`gofmt` output is enforced by CI. Format the files you touch before pushing.

### Static analysis

```bash
make vet        # go vet ./...
make lint       # golangci-lint run ./...
make vulncheck  # govulncheck ./...
make err-lint   # error-convention violations (see docs/errors.md)
```

After touching `token.Keywords` or `resolver.ScalarTypeNames`, run `make generate` to refresh the editor grammar files.

`golangci-lint` is the project's primary linter. Its configuration lives in `.golangci.yml`. Run it before pushing; CI runs it in the `lint` job. `make setup` installs the pinned version (currently v2.14.0); to install it manually:

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0
```

## Development Workflow

The project uses a simple, trunk-based workflow.

```
main
  ↑
  └── feat/fix/docs/perf branch
          ↓
        Pull Request
          ↓
        Review
        ↓
        main
```

1. Create a branch off `main`.
2. Make small, focused commits.
3. Open a pull request against `main` early and mark it as a draft if it is not ready.
4. Address review feedback.
5. Once CI passes and maintainers approve, the branch is merged into `main`.

Branches should be short-lived and focused on a single change. Suggested branch names:

```text
feat/schema-compiler
fix/resolver-panic
perf/lexer-allocations
docs/readme
refactor/container
test/adapters
ci/github-actions
```

### Issue labels

Issues suitable for newcomers carry the `good first issue` label:
small scope, no public API change, verifiable with existing tests.
Maintainers apply the label; contributors pick labeled issues without
asking for assignment.

### Proposing a new battery

Proposing a new battery (new adapter or new service)? Read
[`docs/writing-a-plugin.md`](../docs/writing-a-plugin.md) first: it
defines the plugin shapes, the conformance-kit acceptance gate, and the
naming rules.

## Go Coding Standards

Write idiomatic Go. When in doubt, follow [Effective Go](https://go.dev/doc/effective_go).

- **Formatting.** Always run `make fmt-fix` before committing.
- **Mass refactors.** Repo-wide edits to Go source must use AST-aware tooling (`gofmt -r`, `go/ast`, `gopls rename`) or a script run with a dry run and reviewed before/after diff. Never rewrite struct tags with regex — the `json`/`toml`/`yaml` tags are the wire contract and are guarded by `make check-tags` in CI.
- **Static analysis.** Keep `make vet`, `make lint`, and `make vulncheck` clean.
- **Error handling.** Handle errors explicitly. Return and propagate errors instead of swallowing them. Do not use `log.Fatal` in library code.
- **Meaningful naming.** Prefer clear, descriptive names over short cryptic ones. The name of an exported identifier is part of its public API.
- **Small interfaces.** Favor small interfaces that describe a single behavior. Accept the smallest interface that satisfies the need.
- **Composition.** Prefer composition over inheritance and over wide abstractions. Build behavior from small, composable pieces.
- **Avoid unnecessary abstractions.** Do not add interfaces, packages, or indirection levels until there is a concrete second use case that justifies them.
- **Avoid unnecessary dependencies.** Prefer the standard library. Do not pull in a dependency to save a few lines unless it provides real, maintained value. New dependencies are reviewed carefully.
- **Document exported identifiers.** Every exported type, function, method, and constant must have a doc comment starting with its name. Unexported identifiers do not require comments.
- **Avoid global mutable state.** Globals make code hard to test and reason about. Pass state explicitly, typically through a struct.
- **Context propagation.** Take `context.Context` as the first argument of functions that perform IO or can be cancelled, and pass it through. Never store a `context.Context` in a struct.
- **Resource cleanup.** If a function opens a resource, it must also close it. Prefer `defer` immediately after acquiring the resource.
- **Concurrency safety.** If a type can be used from multiple goroutines, document the guarantees and make them safe. Never export shared mutable state without synchronization.
- **Avoid goroutine leaks.** Every goroutine must have a defined exit path, usually via context cancellation or an explicit stop channel.
- **Avoid data races.** Run the race detector on any code that touches concurrency.
- **Deterministic tests.** Tests must pass reliably. Avoid timing-dependent assertions, `time.Sleep` for synchronization, network access, and dependence on global state or random values unless seeded.

## Testing Standards

Tests are required for new functionality, and bug fixes must normally include a regression test that fails before the fix and passes after. The flake-prevention rules (determinism, process-global state, per-test DB files, `synctest`, fork/exec races) live in [CONTRIBUTING-testing.md](./CONTRIBUTING-testing.md).

Write tests that cover, where relevant:

- normal behavior
- invalid input
- edge cases
- error handling
- concurrency
- cancellation
- resource cleanup
- compatibility behavior

```bash
make test
make test-race
make vet
```

There is no arbitrary coverage threshold. Focus on meaningful coverage of the behavior above rather than chasing a percentage.

## Benchmarks

Performance is a feature of a framework. When a change claims to improve or protect performance, include benchmarks:

```bash
make bench
```

Any PR that claims a significant performance improvement should provide benchmark evidence (before/after output and configuration used). Benchmarks are not required for ordinary bug fixes or documentation changes.

## API Compatibility

zever is a framework: other people build on its public API. Public API changes have downstream consequences.

Before changing anything exported (functions, types, methods, interfaces, constructors, configuration structures, default behavior, error behavior, serialization formats, and extension points), consider:

- **Adding a method to a public interface can break every downstream type that implements it.** This is often a breaking change in Go. Prefer adding new interfaces, or provide a separately named interface when you must extend a contract.
- Adding fields to exported structs can break composite literals that do not use field names.
- Changing a zero value's meaning changes default behavior.
- Changing or removing exported identifiers breaks code that references them.

Keep public API changes minimal and deliberate. Do not make unrelated breaking changes as a side effect of another fix. Public API changes should be called out explicitly in the pull request and reviewed by a maintainer.

The `apidiff` CI gate (`make apidiff`) enforces this for Core-tier modules. An incompatible change in module `M` must be covered by a `**Breaking:**` entry under the changelog's pending-release section that names `M`. The pending section is `## [Unreleased]`, or — between the release cut and the tag push — the top `## [vX.Y.Z] - YYYY-MM-DD` section whose version is not tagged yet: the API-diff baseline stays at the last existing tag, so a break documented in a cut-but-untagged release still passes. Only the top-most released section counts this way, so an older untagged section never absorbs a new break; once `tools/tag-release.sh` creates the tags, the section is released and its entries no longer count. An entry is a `- ` bullet plus its indented continuation lines; it covers `M` when it carries a backticked token that is the module path (`github.com/zenta-dev/zever/core/auth`), the module directory (`core/auth`), a package path under either (`core/auth/jwt`, a trailing `/...` is ignored), the short name (`auth`) or a qualified identifier (`auth.JWTOptions`). A `**Breaking:**` line that names only other modules does not satisfy the gate. A Core-tier module with no `<dir>/v*` baseline tag fails the gate unless it is listed, with a reason and an `expires` date (`YYYY-MM-DD`), in `.github/apidiff-baseline-allowlist.json`; expired entries fail again.

## Versioning

This project follows [Semantic Versioning](https://semver.org/):

```text
MAJOR.MINOR.PATCH
```

- **MAJOR**: breaking changes to the public API.
- **MINOR**: backward-compatible additions.
- **PATCH**: backward-compatible bug fixes.

While the project is below `v1.0.0`, treat `0.x` as a pre-1.0 phase: minor releases may contain breaking changes, and they should still be documented as such in the release notes. Releases are tagged with a `v` prefix (for example `v0.1.0`). See [STABILITY.md](../STABILITY.md) for core vs extended tiers.

Every module (there is no root `go.mod`) shares the lockstep release version with prefixed tags (`tools/zever-lsp/vX.Y.Z`, `adapters/cache/redis/vX.Y.Z`, ...). Modules commit `replace` directives for sibling intra-repo modules (e.g. `dsl`, `core/*`, `orm`, `shared/*`): the module-graph CI gate (`go run ./tools/modgraph --check`, or `make modgraph-check`) requires every intra-repo dependency to resolve inside the workspace, and a published version alone does not satisfy that for unpublished siblings. Keep those `replace` blocks in the committed `go.mod`. The one exception is `cmd/zever` (listed in `noReplaceModules` in `tools/modgraph`): it is the `go install github.com/zenta-dev/zever/cmd/zever@vX` target and the go command rejects a main module that commits `replace` directives, so it pins sibling versions only, `modgraph --check` fails if a `replace` appears, and `go.work` resolves siblings for local development. `make tidy-all` / `tidy-check-all` go through `tools/gotidy.sh`, which injects temporary local replaces for such modules. The release workflow verifies `go install` after the tags are pushed (`tools/verify-go-install.sh`).

## Release checklist (lockstep per-module tags)

Every release ships one number across all modules via `tools/tag-release.sh` (finds every `go.mod`, tags each module as `<module-path>/vX.Y.Z`, e.g. `github.com/zenta-dev/zever/adapters/cache/redis/vX.Y.Z`, in dependency order — `shared/*`, then `core/*` plus `config`/`container`/`orm`/`dsl`/`cmd`, then `adapters/*` and the rest; preview with `--dry-run`; after tagging, confirm every tag exists locally or on origin with `tools/tag-release.sh --verify vX.Y.Z`; without `--push` it only prints the command and never pushes; `--push` creates the tags and publishes them with a single `git push --atomic origin ...` so a rejected ref leaves the remote with none of them; never use `git push --tags`; once pushed, `--verify-proxy vX.Y.Z` asks the module proxy to resolve every module at the tag and the release workflow runs it). There is no root `go.mod`, so the per-module tags carry the release payload; the script also creates the bare `vX.Y.Z` release tag on the same commit, because `.github/workflows/release.yml` triggers on `push: tags: ["v*"]`, which matches a bare tag only — a release without it ships no binaries, no SBOMs and no attestation. The bare tag rides along in the same atomic push, so the workflow cannot start before every module tag is on origin. The `dsl` compiler has no
release-version constant (schema `v1`/`v2` segments are API versions, not
releases) — everything else moves together:

- `CHANGELOG.md`: cut the section, keep `## [Unreleased]` above it. The cut renames the pending `## [Unreleased]` section to `## [vX.Y.Z] - YYYY-MM-DD` and leaves a fresh, empty `## [Unreleased]` section at the top: the next release then has a pending section to cut, the pending fragments always have a home, and the `apidiff` gate can ask for a `**Breaking:**` entry in a section that exists. Deleting the section instead of renaming it strands the next release (the fragments can no longer be compiled and the gate is unsatisfiable); `make changelog-compile-check` (CI: the `changelog` job) fails when the top section is not `## [Unreleased]`. Until `tools/tag-release.sh` creates the tags, `make apidiff` also treats the cut top section as the pending release when its version is the in-development one (`cmd/zever/VERSION`), so a break documented there still satisfies the gate (see [Versioning](#versioning)).
- Installers: `install.sh` (`ZEVER_VERSION_DEFAULT`, header and usage text) and
  `install.ps1` (`$Version` default and help text).
- Root pins: `README.md` (installer URLs), `CITATION.cff`, `cmd/zever/new.go` (`defaultFrameworkVersion`)
  plus `cmd/zever/new_test.go`, `cmd/zever/VERSION` (CLI fallback version; `tools/tag-release.sh` refuses to tag until it matches, and `make version-check` fails when it lags the newest tag; release builds also inject it via `-ldflags -X main.cliVersion`),
  `cmd/zever/README.md`.
- LSP: `tools/zever-lsp/server.go` (`serverVersion`), `tools/zever-lsp/go.mod`
  intra-repo requires + `go.sum` via `go mod tidy`, `tools/zever-lsp/README.md`.
- Editors: `editors/vscode/package.json`, `editors/vscode/README.md`,
  `editors/nvim/README.md` (LSP install pins).
- Docs: `docs/src/content/docs/start/installation.mdx`,
  `docs/src/content/docs/guides/operate/upgrade.mdx`,
  `docs/src/content/docs/guides/extend/editor-setup.mdx`,
  `docs/src/content/docs/reference/cli.mdx`.
- Docs pins: `docs/src/data/landing.ts` (`VERSION`, used in the installer command),
  `tools/zever-mcp/server.go` (`serverVersion`).
- One-shot pinning: after cutting `CHANGELOG.md`, run `make release-prep VERSION=vX.Y.Z` (`tools/release-prep.sh`). It rewrites `cmd/zever/VERSION` and every file listed in `tools/release-pins.txt` (installers, `CITATION.cff`, docs pins, LSP/MCP `serverVersion`, the VS Code extension version), sets every intra-repo `require github.com/zenta-dev/zever/... vX` in every `go.mod` to `vX.Y.Z` (`go run ./tools/modgraph --set-pins`; `replace` directives and `go.work` are untouched, so the not-yet-tagged siblings resolve locally), then runs `make deps-sync`. Add any new file that hard-codes the release version to `tools/release-pins.txt`.
- Pin gate: `tools/check-version.sh` (`make version-check`) fails when any listed pin, the newest `CHANGELOG.md` release section, or any intra-repo `go.mod` require differs from `cmd/zever/VERSION`; `make modgraph-check` also runs `--check-pins`; and `tools/tag-release.sh` (every mode, including `--verify`) refuses to proceed until it passes. Published tags are immutable once the Go module proxy caches them, so a stale sibling pin cannot be fixed afterwards (v0.7.0 shipped with v0.6.1 pins; v0.7.1 supersedes it, see `STABILITY.md`). Verify with `make modgraph-check` and `make deps-sync-check` before tagging.
- Tags: run `tools/tag-release.sh vX.Y.Z` from a clean checkout of main once the bump lands (the tagger skips nested worktrees, harness checkouts, and vendored copies; it creates one `<path>/vX.Y.Z` per module, dependency order, plus the bare `vX.Y.Z` release tag on the same commit). `tools/tag-release.sh --push vX.Y.Z` publishes the module tags and the bare tag in one `git push --atomic`, so `.github/workflows/release.yml` (which fires on the bare `vX.Y.Z` tag) starts only once every module tag is on origin, and a rejected ref leaves the remote with none of them. Verify with `grep -rn` for the old version (excluding
  `CHANGELOG.md` history and third-party `go.sum` lines) before tagging.

## Commit Convention

Use [Conventional Commits](https://www.conventionalcommits.org/) style messages:

```text
<type>: <short description>
```

```text
feat: add schema compiler
fix: prevent resolver panic
perf: reduce lexer allocations
refactor: simplify container wiring
test: add parser regression tests
docs: improve adapter documentation
ci: test against Go 1.27
build: update dependencies
```

Keep messages concise and meaningful. The subject line should complete the sentence "This change will ...". Do not squash unrelated changes into a single commit.

## Changelog

User-facing changes are recorded in [CHANGELOG.md](../CHANGELOG.md) under `## [Unreleased]`. Add entries for new features, behavior changes, fixes, and removals before opening a pull request.

## License

By submitting a contribution to this project, you agree that it is licensed
under the [Apache License, Version 2.0](../LICENSE), without any additional
terms or conditions (inbound contributions are licensed on the same terms as
outbound distributions). No separate contributor license agreement (CLA) is
required.
