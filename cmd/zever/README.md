# zever CLI (`cmd/zever`)

Flags-only toolkit shell for the `zever` module. Every command is an
exact CLI invocation; run `zever --help` for the grouped command map.

Module root: [`../../README.md`](../../README.md).

## Install

Single-module repo (no nested `go.mod` under `cmd/zever`), so install from
the module root path:

```sh
go install github.com/zenta-dev/zever/cmd/zever@v0.3.0
```

Then `zever --help` prints the grouped command map (Scaffolding /
Inspection / Runtime / Database).

## Flags-only UX

- **Bare `zever` (no subcommand)**, on a TTY or not: prints usage to
  stderr and exits `1` with `zever: missing subcommand` — scripts never
  hang waiting on a prompt.
- **Guided prompts for one shot**: `-i` / `--interactive` (global,
  before or after the subcommand) or `ZEVER_INTERACTIVE=1|true|yes` sets
  interactive mode via `huh` guided prompts (`prompt.go`); `zever <command> -h` /
  `zever help <command>` always prints flags without running anything.
- For the full command list, see `zever --help`.

## Non-TTY contract

`run()` with no subcommand prints usage to stderr + `errMissingSubcommand`,
and `main` exits `1`. All errors go to stderr; stdout stays clean for
command output.

## Dev banner / tinker security note

- `zever dev` is local watch-mode only: it prints a
  `zever dev: watching <schemaDir> and <serverEntry> … (Ctrl-C to stop)` line
  to stdout and restarts the server entrypoint on change. Ctrl-C stops the
  watcher and the server.
- `zever tinker` is **development-only**: every CLI start prints
  `WARNING: zever tinker is development-only: it evaluates arbitrary Go
  against a live container, so run it against a development database only.`
  to stderr (see `tinkerDevWarning` in `tinker.go`). The REPL talks
  to the project's real container via the per-project shim
  (`project.tinker_entry`, default `cmd/tinker-shim` — scaffold it with
  `zever generate tinker`); never point it at production data.
