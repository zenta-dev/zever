---
name: zever
description: Use when scaffolding, inspecting, or running a zever Go application: scaffold apps and batteries, validate and compile .zen schemas, run servers and workers, or drive the CLI from scripts. Trigger on requests like "scaffold zever app", "add battery", "check schema", "compile schemas", or "run zever".
---

# zever skill

Zever is a Go multi-module framework: a schema-driven scaffolding compiler (`.zen` DSL) plus swappable backend batteries. This skill covers the CLI surface an agent needs; library design lives in `AGENTS.md` and `docs/`.

## Machine interfaces

Prefer machine output over human output:

- `zever --json <command>`: JSON envelope `{ok, command, exitCode, data?, error?}` on stdout. Exit codes: 0 success, 1 runtime error, 2 flag misuse.
- `zever --help --agent`: machine-readable command catalog (this list, as JSON).
- `compile`, `check`, `explain`, `doctor` emit structured `data` under `--json`.
- MCP server: build `tools/zever-mcp` and register the binary as an MCP stdio server (`zever_compile`, `zever_schema`, `zever_explain`).

## Workflows

### Scaffold and run

```sh
zever new myapp --dry-run   # preview first
zever new myapp
cd myapp && go mod tidy
zever check schema/app.zen --json
zever compile schema/app.zen --backend=zenorm,proto,atlas
zever db migrate --adapter=sqlite --dsn=data/app.db schema/app.zen
zever serve
```

### Add a battery

```sh
zever add cache/redis --dry-run   # preview edits
zever add cache/redis && go mod tidy
```

### Inspect a schema

```sh
zever routes schema/app.zen
zever explain UserService.GetUser schema/app.zen --json
zever compile schema/app.zen --backend=mcp --out ./gen
```

## Safety

- Every mutating command accepts `--dry-run`: preview before writing.
- `zever tinker` evaluates arbitrary Go against a live container: development databases only, never production.
- Never log secrets or raw option maps; errors name services and fields only.

## Commands

- `zever add`: Add one battery to the calling project
- `zever breaking`: Report API-breaking changes between two schema versions
- `zever check-boundaries`: Report cross-module reference violations
- `zever check`: Validate .zen schemas without writing output
- `zever compile`: Compile .zen schemas through backends
- `zever completion bash`: Generate bash completion script
- `zever completion fish`: Generate fish completion script
- `zever completion powershell`: Generate PowerShell completion script
- `zever completion zsh`: Generate zsh completion script
- `zever completion`: Generate shell completion scripts
- `zever config show`: Print the resolved config, redacted
- `zever config`: Inspect resolved config
- `zever db migrate`: Create tables from .zen schemas
- `zever db rollback`: Undo the most recently applied migration statements
- `zever db seed`: Run seed entrypoint (default db/seed)
- `zever db`: Database commands (migrate, rollback, seed)
- `zever dev`: Watch schemas & source; recompile and restart
- `zever docs`: Generate man pages and markdown reference docs
- `zever doctor`: Verify every battery resolves
- `zever explain`: Print an operation's declaration location and summary
- `zever extract`: Extract one module into a standalone service
- `zever fmt`: Format .zen schemas
- `zever generate`: Scaffold modules, entities, jobs, schedules, entrypoints
- `zever graph`: Print Mermaid entity-relation and module diagrams
- `zever help`: Help about any command
- `zever new`: Scaffold a brand new zever application
- `zever outbox dlq`: Inspect and repair failed messages
- `zever outbox purge`: Delete processed history
- `zever outbox status`: Show relay counters
- `zever outbox`: Inspect and repair the outbox dead-letter queue
- `zever queue:work`: Run worker entrypoint (default cmd/worker)
- `zever routes`: List HTTP routes declared by RPCs
- `zever schedule:run`: Alias for queue:work (worker runs scheduler)
- `zever serve`: Run server entrypoint (default cmd/server)
- `zever tinker`: Live container REPL via tinker shim
