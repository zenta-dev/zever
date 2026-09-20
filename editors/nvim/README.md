# zever Neovim support

Neovim support for the `.zen` schema DSL used by `internal/dsl` in
[zenta-dev/zever](https://github.com/zenta-dev/zever).

This subdirectory is a self-contained Neovim "runtime" plugin: point your
plugin manager at it and it behaves like any other filetype plugin.

## What this provides

- **Filetype detection**: `*.zen` files are recognized as filetype `zen`
  (`ftdetect/zen.vim`).
- **Syntax highlighting**: a traditional regex-based Vim syntax file
  (`syntax/zen.vim`) covering the DSL's reserved keywords (`entity`,
  `service`, `job`, `schedule`, `rpc`, `message`, `index`, `enum`,
  `has_many`, `has_one`, `belongs_to`, `many_to_many`, `true`, `false`),
  builtin scalar types (`uuid`, `string`, `int32`, `int64`, `float32`,
  `float64`, `bool`, `timestamp`, `date`, `bytes`, `json`), contextual
  block labels (`http:`, `queue:`, `cron:`, `join_table:`, ...) and
  attribute names (`foreign_key`, `on_delete`, `validate`, `default`,
  `primary`, `unique`, `required`), HTTP verbs, `//` line
  comments, string and duration/number literals, `@attribute` annotations,
  and the `->` operator.
- **Documented LSP client config** (see below) — not auto-loaded, since
  registering a language server is a project/user decision, not something a
  filetype plugin should do on your behalf.

This is filetype support only, out of the box, once installed. LSP
features require the separate setup described below, once wired up
this extension surfaces whatever `zever-lsp` provides:
- diagnostics, completion (with resolve), hover, go-to-definition,
  document symbols, and document formatting
- range formatting, rename (entity, field, job, service, RPC, with a
  pre-rename check), find-references, and document-highlight
- workspace-symbol search, folding-range, semantic-tokens, code-actions,
  inlay-hints (with resolve), and signature-help

## Installation

### lazy.nvim

lazy.nvim has no `rtp` plugin-spec key for exposing a plugin that lives in
a subdirectory of a larger repo. Its documented equivalent is a `config`
function that appends the subdirectory to Neovim's `runtimepath`:

```lua
{
  'zenta-dev/zever',
  config = function(plugin)
    vim.opt.rtp:append(plugin.dir .. '/editors/nvim')
  end,
}
```

(The framework's Go source lives alongside the plugin in one repo, so only
`editors/nvim` is added rather than the repo root.) Double-check this
against current lazy.nvim docs if it changes behavior on your version, as
plugin-manager option surfaces can shift between releases.

### packer.nvim

packer does not have a direct equivalent of lazy.nvim's `rtp` option for
loading a single subdirectory as the plugin root. The common workaround is
`rtp` set as part of the plugin spec's `config`/post-setup, or manually
appending the subdirectory to `runtimepath` yourself:

```lua
use({
  'zenta-dev/zever',
  rtp = 'editors/nvim', -- if your packer version supports this key
})
```

If that key isn't honored by your packer version, fall back to a manual
`runtimepath` append pointing at wherever packer (or you) cloned the repo:

```lua
vim.opt.rtp:append('~/.local/share/nvim/site/pack/packer/start/zever/editors/nvim')
```

Adjust the path to match your actual packer install location. This same
manual `vim.opt.rtp:append(...)` approach works for any plugin manager (or
no plugin manager at all) — point it at a local clone of the repo plus
`/editors/nvim`.

## LSP setup

This plugin does **not** register the `zen` language server for you.
Server registration is a project/user decision (command location, root
detection, capabilities), so add it explicitly in your own Neovim config.

The snippet below uses the modern Neovim LSP API (`vim.lsp.config` +
`vim.lsp.enable`), available in Neovim 0.11+. If you're on an older
Neovim, use `nvim-lspconfig` or the older `vim.lsp.start_client`/autocmd
pattern instead.

```lua
vim.lsp.config('zever_lsp', {
  cmd = { 'zever-lsp' },
  filetypes = { 'zen' },
  root_dir = function(bufnr, on_dir)
    -- .git first (a normal zever checkout); zever.yaml/go.mod next, so a
    -- scaffolded project with no git repo of its own yet still finds its
    -- real root. Falling back to the process's cwd (rather than the
    -- buffer's own directory) points the workspace scan at whatever
    -- directory Neovim happened to be launched from -- often unrelated to
    -- the file being edited -- which silently breaks cross-file features
    -- like go-to-definition into another .zen file.
    local root = vim.fs.root(bufnr, { '.git' })
      or vim.fs.root(bufnr, { 'zever.yaml', 'zever.yml', 'zever.json', 'go.mod' })
      or vim.fs.dirname(vim.api.nvim_buf_get_name(bufnr))

    on_dir(root)
  end,
})
vim.lsp.enable('zever_lsp')
```

Neovim's built-in LSP client does not format on save automatically —
`vim.lsp.buf.format()` must be called explicitly. This is opt-in; omit the
autocmd below for manual-only formatting (`:lua vim.lsp.buf.format()`).

```lua
vim.api.nvim_create_autocmd('BufWritePre', {
  pattern = '*.zen',
  callback = function(args)
    vim.lsp.buf.format({ bufnr = args.buf, timeout_ms = 2000 })
  end,
})
```

**Prerequisite**: the `zever-lsp` binary must be on `$PATH`. It's built
from a sibling package in this repo:

```bash
go install ./tools/zever-lsp
```

Run that from a local clone of the repo so `go install` can resolve the
module.

Without a local clone, install the published release instead:

```bash
go install github.com/zenta-dev/zever/tools/zever-lsp@v0.1.0
```

## Verifying it worked

1. Open a `.zen` file and run `:set filetype?` — it should print
   `filetype=zen`.
2. Confirm syntax colors appear: keywords, types, strings, comments, and
   `@attributes` should each be highlighted distinctly.
3. Once the LSP config above is added and `zever-lsp` is installed and on
   `$PATH`, run `:checkhealth vim.lsp` — it should show `zever_lsp`
   attached to the buffer.
4. With the server attached, introduce a deliberate error in a `.zen` file
   and run `:lua vim.diagnostic.open_float()` to see live diagnostics.

## Maintainer note

The keyword/type/label word lists in `syntax/zen.vim` must be kept in sync
with the sibling TextMate grammar at
`editors/vscode/syntaxes/zen.tmLanguage.json` (built in a parallel PR). A
comment at the top of `syntax/zen.vim` repeats this note.
