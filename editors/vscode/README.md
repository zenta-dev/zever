# Zever DSL for VS Code

Editor support for the [zever](https://github.com/zenta-dev/zever) schema DSL: `.zen` files declaring `entity`/`service`/`job`/`schedule`/`message` blocks (modules are dir-derived, not declared).

## Features

- **Syntax highlighting** for `.zen` files: keywords, scalar types, string/duration/numeric literals, `//` comments, `@attribute` markers, block labels (`http`, `auth`, `permission`, `queue`, `retry`, `cron`, `dispatch`, `join_table`, `foreign_key`, `on_delete`, `validate`, `default`, `primary`, `unique`, `required`) and HTTP verbs (`GET`, `POST`, `PUT`, `PATCH`, `DELETE`).
- **Language server integration** via [`vscode-languageclient`](https://www.npmjs.com/package/vscode-languageclient), wired to an external `zever-lsp` binary over stdio. Once `zever-lsp` is installed and running, this extension surfaces whatever the server provides (verified against `tools/zever-lsp/server.go` capabilities):
  - diagnostics, hover, go-to-definition, document symbols, completion (+resolve) and signature help
  - document formatting and range formatting, code actions
  - prepare-rename + rename (entity, field, job, service, RPC), find-references and document-highlight
  - workspace-symbol search, folding-range, semantic-tokens and inlay hints (+resolve)
- **Format on save**: once `editor.formatOnSave` is enabled for the `zen` language, saving a `.zen` file formats the document through the server. Formatting is syntactic (indentation and token spacing).
- **Schema status**: a `zen` language status item (`$(database) Zen: …`) showing the active document's `entity`/`message`/`service` counts plus the current error-diagnostic count. Scanned client-side from the open document (no extra server requests); turns amber when errors are present.
- **Snippets**: client-side TextMate snippets for `zen` (`snippets/zen.json`, no LSP involvement) — see below.

## Prerequisite: the `zever-lsp` binary

This extension is a **client only**. It does not bundle or auto-install the language server. The recommended path is the pinned one-line installer, which provides `zever-lsp` on `PATH` alongside the CLI:

```bash
curl -fsSL https://raw.githubusercontent.com/zenta-dev/zever/main/install.sh | sh
```

Alternatively, build and install it from a local clone of the `zever` repository:

```bash
git clone https://github.com/zenta-dev/zever.git
cd zever
go install ./tools/zever-lsp
```

Without a local clone, install the published release instead:

```bash
curl -fsSL https://raw.githubusercontent.com/zenta-dev/zever/main/install.sh | sh
```

Make sure the resulting binary is on your `$PATH` (`go install` puts it in `$(go env GOPATH)/bin` by default). If `zever-lsp` cannot be found when the extension activates, you'll see an error notification with this same instruction.

## Development

```bash
cd editors/vscode
npm install
npm run compile   # type-check + emit to out/
npm test          # compile + headless unit tests (node --test over out/*.test.js)
npm run test:grammar  # TextMate snapshot tests (generates/updates test/grammar/*.snap)
```

Then open this `editors/vscode` folder in VS Code and press `F5` to launch an Extension Development Host with the extension loaded. Open any `.zen` file to see syntax highlighting; LSP features additionally require `zever-lsp` on your `$PATH` as described above.

Use `npm run watch` during development to recompile TypeScript on save.

To package locally (no publishing): `npm run package` (requires `@vscode/vsce`), then delete the generated `.vsix` — build artifacts must not be committed.

## Configuration

The extension contributes the following settings (under `zever.*`):

- `zever.trace.server` (string, default `"off"`): trace level for language-server communication. One of `"off"`, `"messages"`, `"verbose"`.
- `zever.serverPath` (string, default `"zever-lsp"`): path to the `zever-lsp` executable used to start the server.
- `zever.inlayHints.implicitModule` (boolean, default `true`): show inlay hints for implicit module references.
- `zever.inlayHints.errorCases` (boolean, default `true`): show inlay hints for error cases.

## Commands

- `Zever: Restart Language Server` (`zever.restartServer`): stops and restarts the language server client. Available from the command palette.

## Schema status item

While a `.zen` file is active, the status bar shows e.g.
`$(database) Zen: 2 entities, 1 message, 1 service, 0 errors`. The
declaration counts are scanned from the open document's text (lines starting
with `entity`/`message`/`service`); the error count comes from the document's
current diagnostics. The item refreshes as you type, switch editors, or when
diagnostics change, and turns amber while the document has errors. With no
`.zen` file active it collapses to `$(database) Zen`.

## Snippets

Client-side TextMate snippets for the `zen` language (wired via
`contributes.snippets`, no language server involved). Type a prefix and press
`Tab`:

| Prefix     | Expands to                              |
|------------|-----------------------------------------|
| `entity`   | `entity` block with `id: uuid @primary` |
| `message`  | `message` block with one field          |
| `service`  | `service` block with one `rpc`          |
| `job`      | `job` block with a `queue:` option      |
| `schedule` | `schedule` block with `cron:`/`dispatch:` |
| `enum`     | `enum` block with two values            |
| `saga`     | `saga` block with one `execute:`/`compensate:` step |
| `relation` | relation field (`has_many`/`has_one`/`belongs_to`/`many_to_many` choice) for use inside an `entity` block |

## Known limitations

- The `zever-lsp` binary is **not bundled** with this extension and is **not auto-installed**. Manual installation on `$PATH` is required, as described above. Bundling/auto-install of the server binary is planned future work.
- End-to-end testing of the LSP-client half depends on the `zever-lsp` server existing on disk; the headless `npm test` suite covers the client lifecycle with a stubbed transport instead.
