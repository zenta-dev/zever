# AI-assisted development

How AI assistance is used in this repo, and how its output is
reviewed. This describes what is actually observable in the repo
today; it introduces no new process.

## What AI assistance covers

Contributors may use AI coding assistants (including agent-style
workflows in isolated branches or worktrees) for the same work any
contributor does: exploring code, drafting changes, writing tests,
and drafting docs. There is no separate AI track: AI-assisted
changes land through the same trunk-based workflow as any other
change (`.github/CONTRIBUTING.md:80`): short-lived branch off
`main`, small focused commits, PR against `main`, review, merge.

## What does not change

- Conventional Commits, one logical change per commit
  (`.github/CONTRIBUTING.md:240`).
- `CHANGELOG.md` entry under `## [Unreleased]` before review for
  user-facing changes (`.github/CONTRIBUTING.md:261`).
- Coding and testing standards apply equally: doc comments on
  exports, small interfaces, no globals, `context.Context` first
  for IO, deterministic tests
  (`.github/CONTRIBUTING.md:128`,
  `.github/CONTRIBUTING-testing.md`).
- Public API changes need explicit maintainer review, enforced in
  part by the `apidiff` gate (`Makefile:91`,
  `.github/CONTRIBUTING.md:182`).

## Review of AI-assisted output

AI-assisted PRs get the same review as human-written ones, plus
extra skepticism where AI output is weakest:

1. **PR description and scope.** Fill out
   `.github/pull_request_template.md` (problem, change, related
   issue, type). Keep the diff small enough to review; split
   unrelated changes.
2. **Correctness over plausibility.** AI drafts can look right while
   misunderstanding ownership (for example resolving an adapter in
   the wrong layer instead of the container, or adding a method to a
   public interface instead of a new one). Check layer rules in
   `docs/architecture.md`, `CLAUDE.md`, and `AGENTS.md`.
3. **Verify references.** Every `file:line` reference in docs and
   every claimed behavior must be checked against the tree (grep /
   read the file). Do not merge plausible-but-stale paths.
4. **Run the gates.** At minimum `make fmt`, `make vet`,
   `make test` for touched modules; the full local mirror is
   `make check` (`AGENTS.md:5`). CI is the backstop, not the first
   check: `fmt`, `lint`, `test`, `test-race`, conformance
   (`.github/workflows/ci.yml:207`), `apidiff`, `vulncheck`.
5. **Security and secrets.** Never paste real secrets into prompts or
   commit AI-suggested credentials. Redaction rules
   (`config/redact.go:257`) apply to logs and to PR output alike.
   Report vulnerabilities privately per `.github/SECURITY.md`;
   do not file them as public issues, AI-assisted or otherwise.

## Practical guidance for agent runs

- Work in an isolated branch or worktree; never commit directly to
  `main`, never force-push shared branches.
- Keep to the task scope (for docs tasks: docs only). Unrelated
  `go.mod`/`go.sum` churn from dependency bumps belongs in its own
  change (`make deps-sync` discipline in `AGENTS.md`).
- When the run finishes, report: branch, PR URL, CI status, diff
  stat, what was not verified, and what needs human review.
  Anything unverified is marked `TODO(verify)`, never asserted.
