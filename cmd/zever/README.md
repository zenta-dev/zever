# zever CLI (`cmd/zever`)

Flags-only toolkit shell for the `zever` module. Every command is an
exact CLI invocation; run `zever --help` for the grouped command map.

Module root: [`../../README.md`](../../README.md).

## Install

Recommended path: the pinned one-line installer. It installs the `zever`
CLI plus the matching `zever-lsp` into `~/.local/bin` (Go 1.27+ is required
but not installed by the script). Re-running it upgrades an existing install
in place (older asks to confirm, same version skips, newer asks to confirm a
downgrade; non-TTY shells need `--yes`):

```sh
curl -fsSL https://raw.githubusercontent.com/zenta-dev/zever/main/install.sh | sh
```

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/zenta-dev/zever/main/install.ps1 | iex
```

If PowerShell blocks the script, download it first, then run
`Unblock-File install.ps1` or invoke it with
`powershell -ExecutionPolicy Bypass -File install.ps1`.

Do not use `go install github.com/zenta-dev/zever/cmd/zever@version`: the
repo commits `replace` directives, so `go install <module>@version` does not
work for zever modules. Use the installer above or a prebuilt release binary
instead.

After install, `zever upgrade` self-updates (`--check` to preview,
`--version` to pin a target) and `zever uninstall` removes both binaries
(`--keep-lsp`, `--dry-run`).

Then `zever --help` prints the grouped command map (Scaffolding /
Inspection / Runtime / Database / Operations).

## Global flags and exit codes

- `--quiet`: suppress hints/tips on stderr; errors still print.
- `--no-color`: disable styled output (also honored via `NO_COLOR` /
  `TERM=dumb` ambient env).
- `--json`: machine-readable JSON envelope output (also `ZEVER_JSON=1`).
- `-i`, `--interactive`: guided prompts where supported (also
  `ZEVER_INTERACTIVE=1`).
- `-V`: print the CLI version (alias for `--version`).
- `--help --agent`: agent-friendly command map.
- Exit codes: `0` success, `1` runtime error, `2` flag misuse
  (bad flags print the error to stderr with no usage dump).
- `--help` output goes to stdout; errors and bare-invocation usage go
  to stderr.

## Generated docs

- `zever completion <bash|zsh|fish|powershell>` prints that shell's
  completion script to stdout; redirect into your completions dir, e.g.
  `zever completion bash > /etc/bash_completion.d/zever`.
- `zever docs --dir <dir>` writes one man page plus one markdown file
  per command into `<dir>`; build stripped release binaries with
  `make build-release` (`-trimpath -ldflags="-s -w"`).

## Flags-only UX

- **Bare `zever` (no subcommand)**, on a TTY or not: prints usage to
  stderr and exits `1` with `zever: missing subcommand` — scripts never
  hang waiting on a prompt.
- **Guided prompts for one shot**: `-i` / `--interactive` (global,
  before or after the subcommand) or `ZEVER_INTERACTIVE=1|true|yes` sets
  interactive mode via `huh` guided prompts (`prompt.go`); `zever <command> -h` /
  `zever help <command>` always prints flags without running anything.
- **`zever new` battery picker (opt-in)**: without `--interactive` the command
  never prompts and scaffolds the floor set (`log`+`router`) plus whatever
  `--batteries a,b` and `--adapters b=a` select; `-y` / `--yes` skips all
   prompts, `--list-batteries` prints the battery/default-adapter table and
   exits.
- `zever new --module PATH`: Go module path for the new project (default:
  the app name itself).
- `zever new --dir PATH`: output directory (default `./<name>`).
- `zever new --framework-version V`: depend on a published zever version
  instead of a local replace directive.
- `zever new --force`: scaffold into a non-empty directory anyway.
- For the full command list, see `zever --help`.

## `zever new` Docker scaffold

Every `zever new` project ships a reference `Dockerfile` + `.dockerignore`:

- Multi-stage build: `golang:<ver>-bookworm` builder compiles
  `./cmd/server`, then `gcr.io/distroless/static-debian12:nonroot`
  runs it (a `busybox` stage supplies `/usr/bin/wget` for the
  healthcheck only).
- Runs as non-root (`USER nonroot:nonroot`), exposes `8080` (HTTP)
  + `9090` (gRPC), `ENTRYPOINT ["/app/server"]`, and healthchecks
  `GET /healthz` (the same endpoint the generated server template
  serves; `/readyz` gates on the container readiness aggregate —
  a DB ping plus, when `outbox.stall_readiness` is set, the
  relay's stall flag).
- Copies the binary + `zever.yaml` only. Secrets are never baked
  into the image — pass config at runtime via environment, e.g.
  `docker run -e DB_DSN=... app`.

```sh
docker build -t app .
docker run --rm -p 8080:8080 -p 9090:9090 app
```

When a picked battery needs external infra (`db=postgres`,
`search=postgres`, `vectorstore=pgvector`, or any `*=redis` battery),
`zever new` also writes `compose.yaml`: `app` (build `.`) plus `db`
(`postgres:16-bookworm`) and/or `redis` (`redis:7-alpine`) with
health-gated `depends_on`, persistent volumes, and env-carried secrets
(`DB_DSN`, `<BATTERY>_URL`, `DB_PASSWORD`). All-local picks emit no file.

```sh
DB_PASSWORD=... docker compose up --build
```

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
