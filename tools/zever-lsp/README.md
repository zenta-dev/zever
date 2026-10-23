# zever-lsp

LSP 3.18 language server for the [zever schema DSL](../..), built on the
`go.lsp.dev` stack (`jsonrpc2` + `protocol` + `uri`). It compiles your `.zen`
workspace in-process and serves diagnostics, navigation, completions, and
edits to any LSP-capable editor over stdio.

## Install

Recommended path: the pinned one-line installer. It installs the `zever`
CLI plus the matching `zever-lsp` into `~/.local/bin` (Go 1.27+ is required
but not installed by the script). `zever-lsp` is always matched to the CLI
version — it has no version flag of its own:

```sh
curl -fsSL https://raw.githubusercontent.com/zenta-dev/zever/main/install.sh | sh
```

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/zenta-dev/zever/main/install.ps1 | iex
```

Do not use `go install github.com/zenta-dev/zever/tools/zever-lsp@version`:
the repo commits `replace` directives, so `go install <module>@version` does
not work for zever modules. Use the installer above or a prebuilt release
binary instead.

Make sure `~/.local/bin` is on your `PATH`, then point your editor at the
`zever-lsp` binary.

## Capabilities

Declared in `server.go`'s `Initialize` and implemented in this module:

| Area | Methods |
| --- | --- |
| Sync | `textDocument/didOpen`, `textDocument/didChange` (full), `textDocument/didSave`, `textDocument/didClose`, `workspace/didChangeWatchedFiles` |
| File operations | `workspace/didCreateFiles`, `workspace/didDeleteFiles` (surgical `diskCache` updates with no full rescan; `didChangeWatchedFiles` stays the coarse fallback) |
| Diagnostics | `textDocument/publishDiagnostics` (debounced, per-file; clean files get an empty array; each diagnostic carries a stable `zen-###` code — see [Diagnostic codes](#diagnostic-codes) below) |
| Symbols | `textDocument/documentSymbol`, `workspace/symbol` (resolved schema preferred, per-file AST fallback) |
| Navigation | `textDocument/definition`, `textDocument/declaration` (alias for definition — every declaration is its own declaration site), `textDocument/typeDefinition` (the named type under the cursor: a scalar or unknown type is honestly null), `textDocument/references`, `textDocument/documentHighlight`, `textDocument/foldingRange`, `textDocument/selectionRange`, `textDocument/documentLink` (with resolve) |
| Hover | `textDocument/hover` |
| Completion | `textDocument/completion` (triggers `@`, `:`, ` `), `completionItem/resolve` |
| Signature help | `textDocument/signatureHelp` (triggers `(`, `,`) |
| Formatting | `textDocument/formatting`, `textDocument/rangeFormatting`, `textDocument/onTypeFormatting` |
| Actions | `textDocument/codeAction` (quick fixes for unknown scalar types and unknown relation targets) |
| Rename | `textDocument/prepareRename`, `textDocument/rename` (entities, fields, jobs, services, RPCs) |
| Code lens | `textDocument/codeLens`, `codeLens/resolve` (per-declaration `N reference(s)` counts; info-only, gated by `zever.codelens.references`) |
| Semantic tokens | `textDocument/semanticTokens/full` (with delta), `textDocument/semanticTokens/range` (the `definition` modifier marks declaration identifiers) |
| Inlay hints | `textDocument/inlayHint`, `inlayHint/resolve` (implicit-module, `errors:`-case, and `paramNames:` parameter-name annotations, per-hint-family configurable — see [Configuration](#configuration) below) |
| Configuration | `workspace/didChangeConfiguration` (`zever.*` settings, see [Configuration](#configuration) below) |
| Refresh | `workspace/codeLens/refresh`, `workspace/inlayHint/refresh` (pushed when `didChangeConfiguration` flips that family, only when the client advertised `refreshSupport`; semantic tokens never refresh) |
| Lifecycle | `initialize`, `initialized`, `shutdown`, `exit`, `$/setTrace` |

## Non-goals

`textDocument/prepareTypeHierarchy` (and `typeHierarchy/supertypes` /
`subtypes`) is deliberately absent: the `.zen` DSL has nominal types with no
inheritance. Entities, messages, enums, and sagas are unrelated nominal
declarations (`dsl/ast`); relations are associations between entities, not
subtyping (`dsl/ir`'s `Relation` holds a resolved target pointer, not a
supertype); and `ir.TypeRef` is a reference union (exactly one of entity /
message), not a subtype bound. There is no supertype or subtype to report,
so the server answers no type-hierarchy capability rather than inventing one.

### Position encoding

The server computes LSP character offsets as UTF-8 byte offsets, so in
`initialize` it picks `utf-8` exactly when the client offers it in
`general.positionEncodings` and then echoes `capabilities.positionEncoding:
"utf-8"`. When the client does not offer it, the capability is omitted and
the protocol's implicit `utf-16` default stays in place (semantic tokens
are UTF-16-exact regardless, per spec construction). Identifiers in `.zen`
schemata are ASCII, so both encodings agree on every position the server
hands out.

## Configuration

Read from `workspace/didChangeConfiguration`, nested under the sole `zever`
section. A well-formed payload is a whole replacement (families absent from
it fall back to their defaults); a malformed or non-object payload leaves
the current settings untouched. Per-key values of the wrong type are
ignored and keep their default.

```json
{
  "zever": {
    "inlayHints": {
      "implicitModule": true,
      "errorCases": true,
      "paramNames": false
    },
    "codelens": {
      "references": true
    },
    "diagnostics": {
      "debounceMs": 250
    }
  }
}
```

| Key | Default | Meaning |
| --- | --- | --- |
| `zever.inlayHints.implicitModule` | `true` | `(module: public)` hints on declarations resolving to the implicit public module (shown only when the schema mixes public and named modules) |
| `zever.inlayHints.errorCases` | `true` | status/gRPC-name annotations on `errors: { ... }` case names |
| `zever.codelens.references` | `true` | `N reference(s)` lenses on top-level declarations (entities, messages, services, RPCs, jobs, schedules, sagas, enums) |
| `zever.inlayHints.paramNames` | `false` | `name:` annotations on positional `dispatch: Job(...)` call arguments (skipped for already-named arguments and same-name single-argument calls) |
| `zever.diagnostics.debounceMs` | `250` | how long an edit burst must settle before recompiling (values ≤ 0 keep the default) |

## Diagnostic codes

Every published diagnostic that matches a known class carries a stable
`code` (plus a `codeDescription.href` linking here), classified
deterministically from the message text (`diagnosticClasses` in
`diagnostics.go`). Codes never move between classes across releases, so
editor filters stay meaningful. A message matching nothing carries no code
rather than an invented one.

| Code | Class |
| --- | --- |
| `zen-001` | unknown field type (`unknown field type`) |
| `zen-002` | unknown relation target (`references unknown entity`) |
| `zen-003` | duplicate declaration (`duplicate ...`) |
| `zen-004` | cross-module reference (`... different module ...`, `crosses module boundary`) |
| `zen-005` | unknown `@attribute` |
| `zen-006` | unknown identifier outside its vocabulary (`unknown ...`) |
| `zen-007` | parse syntax error (`expected ...`) |
| `zen-008` | invalid literal or value (`invalid ...`, `out of range`, `nesting depth exceeded`) |

## Editor wiring

Neovim (built-in LSP, `vim.lsp.config`):

```lua
vim.lsp.config('zever-lsp', {
  cmd = { 'zever-lsp' },
  filetypes = { 'zen' },
  root_markers = { '.git' },
})
vim.lsp.enable('zever-lsp')
```

VS Code (`clientOptions` in your extension's `serverOptions`):

```ts
const serverOptions: ServerOptions = {
  command: 'zever-lsp',
  transport: TransportKind.stdio,
};
const clientOptions: LanguageClientOptions = {
  documentSelector: [{ scheme: 'file', language: 'zen' }],
};
```

## Transport

Stdio only. The server reads JSON-RPC from stdin and writes it to stdout,
so **stdout must stay clean** — all logging (`zever-lsp: ...`) goes to
stderr. Wrapping the binary in a script that prints to stdout will corrupt
the protocol stream.

## Troubleshooting

- `zever-lsp: command not found` — the binary is not on `PATH`. Re-run the
  installer above and check `~/.local/bin` is on `PATH`.
- No diagnostics / nothing happens — open a `.zen` file and confirm the
  client attached (`:LspInfo` in Neovim, Output > zever-lsp in VS Code);
  `DidChangeWatchedFiles` only rescans the workspace root from `initialize`,
  so opening the right folder matters.
- Garbled responses — something is writing to the server's stdout. Logs go
  to stderr by design; check for wrapper scripts or editor plugins that
  merge the streams.
- There is no `--version` (or any) CLI flag: the server speaks LSP over
   stdio only. The version (`0.8.0` in `server.go`) is reported in the
  `initialize` result's `serverInfo`.

## The DSL itself

The language this server speaks is documented in the repo root:
[../..](../../). Grammar, resolver rules, and the `zever` CLI live there;
this directory contains only the LSP transport and feature handlers.
