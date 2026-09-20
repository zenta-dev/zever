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

## Getting Started

### Prerequisites

Required:

- Go 1.27 or newer (this project uses `go 1.27.0` in `go.mod`)

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
```

`golangci-lint` is the project's primary linter. Its configuration lives in `.golangci.yml`. Run it before pushing; CI runs it in the `lint` job. `make setup` installs the pinned version (currently v2.13.2); to install it manually:

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2
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

## Go Coding Standards

Write idiomatic Go. When in doubt, follow [Effective Go](https://go.dev/doc/effective_go).

- **Formatting.** Always run `make fmt-fix` before committing.
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

Tests are required for new functionality, and bug fixes must normally include a regression test that fails before the fix and passes after.

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

## Versioning

This project follows [Semantic Versioning](https://semver.org/):

```text
MAJOR.MINOR.PATCH
```

- **MAJOR**: breaking changes to the public API.
- **MINOR**: backward-compatible additions.
- **PATCH**: backward-compatible bug fixes.

While the project is below `v1.0.0`, treat `0.x` as a pre-1.0 phase: minor releases may contain breaking changes, and they should still be documented as such in the release notes. Releases are tagged with a `v` prefix (for example `v0.1.0`).

Nested modules (`tools/zever-lsp`) are versioned independently with prefixed tags (`tools/zever-lsp/v0.1.0`) and must `require` a published root version — never a committed `replace`. To iterate locally against working-tree root changes, use a temporary replace and revert it before committing:

```bash
cd tools/zever-lsp && go mod edit -replace github.com/zenta-dev/zever=../.. && go test ./...
git checkout -- tools/zever-lsp/go.mod tools/zever-lsp/go.sum
```

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
