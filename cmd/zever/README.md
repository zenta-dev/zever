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

From v0.7.2 on you can also build from source with
`go install github.com/zenta-dev/zever/cmd/zever@vX.Y.Z` (`cmd/zever/go.mod`
commits no `replace` directives and pins every sibling module). v0.7.1 and
earlier tags cannot be installed this way; use the installer above or a
prebuilt release binary for those.

After install, `zever upgrade` self-updates (`--check` to preview,
`--version` to pin a target, `--project` to upgrade the current project's
`go.mod` instead of the binary) and `zever uninstall` removes both binaries
(`--keep-lsp`, `--dry-run`).

Then `zever --help` prints the grouped command map (Start / Build / Data /
Run & ops, plus the additional commands).

## Global flags and exit codes

- `--quiet`: suppress hints/tips on stderr; errors still print.
- `--no-color`: disable styled output (also honored via `NO_COLOR` /
  `TERM=dumb` ambient env).
- `--json`: machine-readable JSON envelope output (also `ZEVER_JSON=1`).
  Structured `data` is emitted by the inspection commands (`compile`,
  `check`, `explain`, `doctor`, `routes`, `config show`, `graph`,
  `breaking`, `check-boundaries`) plus `outbox`, `upgrade`,
  `uninstall`, and `build`; every other command emits a JSON error envelope on
  failure. `doctor --json` reports `ok:false` with a nonzero exit when
  any battery fails.
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
  `--with a,b=c` selects (batteries with optional adapter pins; it replaces
  the older `--batteries a,b` and `--adapters b=a`); `-y` / `--yes` skips all
   prompts, `--list-batteries` prints the battery/default-adapter table and
   exits. `--profile minimal|api|full` is a deprecated alias for `--with`.
- **`zever compile --check`**: verifies the committed `generated/` tree
  matches a fresh compile (exit `1` naming `missing`/`differs`/`extra`
  files, writes nothing) — the CI drift gate for projects that commit
  generated output. A difference in the stamped CLI version alone is only a
  note unless `--strict` is passed. Every scaffolded
  `.github/workflows/ci.yml` regenerates from the schema and then runs
  `--check`.
- **`zever add <battery>[/<adapter>]`**: edits `internal/app/wiring.go` with
  `go/ast` (a `Ref` var plus a `container.Provide` entry), appends the
  battery's keys to `.env.example`, adds the core and adapter requires to
  `go.mod`, then runs `go get` and `go mod tidy`. `--name N` wires a named
  instance, `--uses R` records a boot-order edge, `--diff` prints the diff
  and changes nothing, and `--yes` applies without prompting.
- `zever new --module PATH`: Go module path for the new project (default:
  the app name itself).
- `zever new --dir PATH`: output directory (default `./<name>`).
- `zever new --framework-version V`: depend on a published zever version
  instead of a local replace directive.
- `zever new --force`: scaffold into a non-empty directory anyway.
- **Local-checkout replace set**: when the scaffold resolves against a
  framework checkout (`ZEVER_FRAMEWORK_PATH` or the auto-detected
  checkout), the generated `go.mod` replaces every module of
  the checkout's module graph — the batteries (`core/<battery>`), the
  adapters (`adapters/<battery>/<adapter>`), the shared helpers and
  everything they require inside the repository — not just the modules the
  project imports. The set is derived from the checkout's own `go.mod`
  files, so a module the framework adds later is covered without touching
  the CLI, and the project's module graph never reaches the module proxy
  for a sibling module (which cannot serve a version that is not published
  yet).
- For the full command list, see `zever --help`.

## Project layout keys

The six `ZEVER_*` keys move the project's layout — where it keeps its `.zen`
schemas (`ZEVER_SCHEMA_DIR`, default `schema`), its generated code
(`ZEVER_GENERATED_DIR`, default `generated`), and the entrypoint packages
the launcher commands shell out to (`ZEVER_SERVER_ENTRY`, default
`cmd/server`; `ZEVER_WORKER_ENTRY`, default `cmd/worker`;
`ZEVER_TINKER_ENTRY`, default `cmd/tinker-shim`; `ZEVER_SEED_ENTRY`, default
`db/seed`).

They resolve with the framework's precedence — defaults < `.env` <
`.env.local` < the process environment — out of the working directory the
CLI treats as the project root (the directory holding `go.mod`). A key
therefore belongs in the `.env` at the project root, or in the `.env.local`
next to it, or in the process environment; no config file is ever read.

## `zever new` contributor tooling

Every `zever new` project also ships a `Makefile` (`generate`, `build`,
`test`, `vet`, `fmt`, `lint`, `run`, `docker`, `help`), a golangci-lint v2
`.golangci.yml`, an `.editorconfig` and `.github/dependabot.yml` (weekly,
grouped Go modules, GitHub Actions and Docker updates). They are plain files
the project owns; zever never rewrites them.

## `zever new` Docker scaffold

Every `zever new` project ships a reference `Dockerfile` + `.dockerignore`:

- Multi-stage build: `golang:<ver>-trixie` builder compiles
  `./cmd/server`, then `gcr.io/distroless/static-debian13:nonroot`
  runs it (a `busybox` stage supplies `/usr/bin/wget` for the
  healthcheck only).
- Carries OCI image labels (`org.opencontainers.image.title`, `version`,
  `revision`, `source`) fed by `--build-arg TITLE|VERSION|REVISION|SOURCE`.
- Runs as non-root (`USER nonroot:nonroot`), exposes `8080` (HTTP)
  + `9090` (gRPC), `ENTRYPOINT ["/app/server"]`, and healthchecks
  `GET /healthz` (the same endpoint the generated server template
  serves; `/readyz` gates on the container readiness aggregate —
  a DB ping plus, when `outbox.stall_readiness` is set, the
  relay's stall flag).
- Copies the binary only. Secrets are never baked
  into the image — pass config at runtime via environment, e.g.
  `docker run -e DB_DSN=... app`.

```sh
docker build -t app .
docker run --rm -p 8080:8080 -p 9090:9090 app
```

When a picked battery needs external infra (`db=postgres`, `search=db`,
`vectorstore=db`, `workflow=db`, or any `*=redis` battery),
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
  (`ZEVER_TINKER_ENTRY` env key, default `cmd/tinker-shim` — scaffold it
  with `zever generate tinker`); never point it at production data.
