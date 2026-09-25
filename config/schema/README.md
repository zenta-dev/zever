# Config schema

`zever.schema.json` is the canonical JSON Schema (draft 2020-12) describing
the shape of `zever.yaml` / `zever.yml` / `zever.json` — every one of
`config.Config`'s 34 battery services, their adapter enums, and their typed
`Options` fields, hand-authored against the real Go structs and kept
strict to match `config/decode.go`'s `DisallowUnknownFields` behavior
(unknown services and unknown option fields are schema errors, exactly as
they are loader errors).

`zever.schema.yaml`, `zever.schema.yml`, and `zever.schema.toml` are the
same schema document, mechanically re-serialized into those formats for
tooling that only accepts a schema file in its own format. All four are
structurally identical after decoding — verified by round-tripping each
one back to a generic value and diffing against the canonical JSON at
generation time. Edit `zever.schema.json` only; regenerate the other three
from it rather than hand-editing them, to avoid drift.

**Important:** `zever.toml` is not itself a loadable zever config format
today — `config/decode.go` only accepts `.yaml`, `.yml`, and `.json`
extensions. `zever.schema.toml` exists for format-parity and TOML-aware
tooling only; it does not mean you can point `zever`'s config loader at a
`.toml` file.

**Also note:** `time.Duration` fields (`timeout`, `ttl`, `max_conn_lifetime`,
etc.) are plain integer nanoseconds in JSON/YAML/TOML *files* — e.g.
`5s` is `5000000000`. Go duration strings like `"5s"` only work for
environment-variable overrides (`DB_MAXCONNLIFETIME=5s`), never inside a
config file itself.

## Wiring into your editor

**YAML** — add a modeline at the top of your `zever.yaml` (works with the
[YAML Language Server](https://github.com/redhat-developer/yaml-language-server)
extension in VS Code, Neovim, etc.):

```yaml
# yaml-language-server: $schema=./path/to/zever.schema.json
```

**JSON** — don't add an inline `$schema` key to `zever.json` itself: file
decoding is strict, `$schema` isn't a registered service name, and it
would be rejected as an unknown top-level key. Use your editor's
schema-association setting instead, e.g. VS Code's `json.schemas` in
`settings.json`:

```json
{
  "json.schemas": [
    { "fileMatch": ["zever.json"], "url": "./path/to/zever.schema.json" }
  ]
}
```

**TOML** — [Taplo](https://taplo.tamasfe.dev/) (the `even-better-toml`
VS Code extension, and the standalone `taplo` LSP) reads a `#:schema`
directive at the top of the file:

```toml
#:schema ./path/to/zever.schema.toml
```

(Remember: this only lints a `.toml` file shaped like `zever.yaml` — it
does not make `zever` itself able to load one.)
