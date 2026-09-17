# zever CLI (`cmd/zever`)

TUI-first toolkit shell for the `zever` module. Bare `zever` on a TTY drops
into the interactive dashboard; every dashboard row maps to an exact
equivalent CLI invocation, and every screen shows that CLI line so what you
learn in the TUI transfers to scripts.

Module root: [`../../README.md`](../../README.md).

## Install

Single-module repo (no nested `go.mod` under `cmd/zever`), so install from
the module root path:

```sh
go install github.com/zenta-dev/zever/cmd/zever@latest
```

Then `zever --help` prints the grouped command map (Scaffolding /
Inspection / Runtime / Database).

## TUI-first UX

- **Bare on a TTY** (`zever`, stdin is a terminal): opens the dashboard
  (`runDashboard` in `dashboard.go`). One `tea.Program` runs a shell model:
  picking an entry swaps the screen in, `esc` pops back to the dashboard
  (`tui.BackMsg` / `tui.CanceledMsg` / `tui.LogBackMsg`), `esc` at the
  dashboard root quits. Clean quit exits `0`; a dashboard failure exits `1`.
- **Bare off a TTY** (pipes, CI): prints usage to stderr and exits `1`
  with `zever: missing subcommand` — the non-TTY contract, so scripts never
  hang waiting on a prompt.
- **Guided prompts for one shot**: `-i` / `--interactive` (global,
  before or after the subcommand) or `ZEVER_INTERACTIVE=1|true|yes` sets
  interactive mode; `zever <command> -h` / `zever help <command>` always
  prints flags without running anything.

## Keys

Single shared keymap (`tui.DefaultKeymap`); the dashboard footer shows it:

| Key              | Action       |
|------------------|--------------|
| `j` / `down`     | move down    |
| `k` / `up`       | move up      |
| `enter`          | select / run |
| `esc`            | back / pop   |
| `ctrl+c`         | quit         |
| `?`              | help overlay |
| `/`              | filter list  |

Footer text: `j/k move · enter select · / filter · ? help · esc back · ctrl+c quit`.
Set `ZEVER_NO_HINT=1` to silence the `did you mean …` / tip hints.

## Screen map (dashboard row → equivalent CLI)

Wired entries come from `RegisterScaffoldScreens` + `RegisterInspectScreens`
+ `RegisterRuntimeScreens` + `RegisterDatabaseScreens` (`screen_common.go`,
`screen_inspect.go`, `screen_runtime.go`, `screen_database.go`); unknown
`Screen` names fail with a `zever: unknown dashboard screen …` error, never
a panic.

| Group    | Dashboard item  | Equivalent CLI          |
|----------|-----------------|-------------------------|
| Scaffold | `new`           | `zever new`             |
| Scaffold | `generate`      | `zever generate`        |
| Scaffold | `extract`       | `zever extract`         |
| Inspect  | `compile`       | `zever compile`         |
| Inspect  | `check`         | `zever check`           |
| Inspect  | `breaking`      | `zever breaking`        |
| Inspect  | `fmt`           | `zever fmt`             |
| Inspect  | `doctor`        | `zever doctor`          |
| Inspect  | `routes`        | `zever routes`          |
| Inspect  | `explain`       | `zever explain`         |
| Inspect  | `check-boundaries` | `zever check-boundaries` |
| Inspect  | `graph`         | `zever graph`           |
| Runtime  | `serve`         | `zever serve`           |
| Runtime  | `dev`           | `zever dev`             |
| Runtime  | `queue:work`    | `zever queue:work`      |
| Runtime  | `schedule:run`  | `zever schedule:run` (same screen as `queue:work`: the worker runs the scheduler in-process) |
| Runtime  | `tinker`        | `zever tinker`          |
| Database | `migrate`       | `zever db migrate`      |
| Database | `rollback`      | `zever db rollback`     |
| Database | `seed`          | `zever db seed`         |

Screens mirror their CLI cores: `compile` defaults to all backends and
`./generated` out dir, `fmt` defaults to list-only, `migrate`/`rollback`
mask the DSN (`--dsn ***` in every displayed CLI line, password-masked
input, scrubbed output).

## Non-TTY fallback contract

`run()` with no subcommand: TTY → `errInteractive` (dashboard); otherwise
usage to stderr + `errMissingSubcommand`, and `main` exits `1`. All errors
go to stderr; stdout stays clean for command output.

## Dev banner / tinker security note

- `zever dev` is local watch-mode only: it prints a
  `zever dev: watching <schemaDir> and <serverEntry> … (Ctrl-C to stop)` line
  to stdout and restarts the server entrypoint on change. Ctrl-C stops the
  watcher and the server.
- `zever tinker` is **development-only**: every CLI start prints
  `WARNING: zever tinker is development-only: it evaluates arbitrary Go
  against a live container, so run it against a development database only.`
  to stderr (see `tinkerDevWarning` in `tinker.go`), and the TUI tinker
  screen banners the same warning before anything evaluates. The REPL talks
  to the project's real container via the per-project shim
  (`project.tinker_entry`, default `cmd/tinker-shim` — scaffold it with
  `zever generate tinker`); never point it at production data.
