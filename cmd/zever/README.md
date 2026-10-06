# zever CLI (`cmd/zever`)

Flags-only toolkit shell for the `zever` module. Every command is an
exact CLI invocation; run `zever --help` for the grouped command map.

Module root: [`../../README.md`](../../README.md).

## Install

Multi-module repo (`cmd/zever` is its own module; batteries live in `core/<b>` with the `Register`/`Open` registry, adapters in `adapters/<b>/<a>` each with `Register()`, helpers in `shared/*`), so install the CLI module at a pinned version:

```sh
go install github.com/zenta-dev/zever/cmd/zever@v0.5.3
```

Then `zever --help` prints the grouped command map (Scaffolding /
Inspection / Runtime / Database).

## Global flags and exit codes

- `--quiet`: suppress hints/tips on stderr; errors still print.
- `--no-color`: disable styled output (also honored via `NO_COLOR` /
  `TERM=dumb` ambient env).
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
