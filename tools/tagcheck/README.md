# tagcheck — struct-tag wire-format guard

`tagcheck` is an AST guard that keeps the JSON wire contract of the repo
stable. It runs in CI through `make check-tags`.

The guard exists because the v0.8.0 env-only config migration silently
stripped 169 `json` struct tags (commit `809a1909`) plus 8 more in
`core/storage/policy.go` (commit `64426337`). With the tag gone,
`encoding/json` falls back to the Go field name, so every consumer of that
payload read a key that no longer existed. The tags were restored in
`b2799b2f`; this guard makes the regression impossible to repeat silently.

## Rules

| rule | what it catches |
|---|---|
| `empty-tag` | a struct field whose tag is present but empty (` `` ` or whitespace): `encoding/json` falls back to the Go field name |
| `forbidden-tag` | a `yaml:`/`toml:` tag outside `tag_allowlist.txt`; the v0.8.0 migration removed yaml/toml serialization deliberately |
| `golden-json` | an exported field of a registered wire struct that lost its `json` tag |
| `golden-key` | an exported field of a registered wire struct whose `json` key differs from the pinned one, whose key is empty, that is dropped from the wire (`json:"-"`), or that came back onto the wire |
| `golden-missing` | a pinned wire key the struct no longer exposes (the field was deleted, or renamed together with its key) |
| `golden-nofields` | a registered wire struct whose keys are not pinned: the golden field data was not regenerated |
| `unregistered-wire` | a struct that exposes json keys but is not registered as a wire struct |
| `golden-removed` | a registered wire struct that disappeared from the tree |
| `no-config-file` | a non-test Go file names a retired project config file (`zever.json`, `zever.yaml`, `zever.yml`, `zever.toml`) — in a string literal, in a constant expression that builds the name, or in a comment |

Files that are skipped: `_test.go` files, generated files (a `DO NOT EDIT`
marker in a leading comment), and the `.git`, `.worktrees`, `.claude`,
`vendor`, `node_modules` and `generated` directories.

## Data files

| file | shape | meaning |
|---|---|---|
| `golden_wire_structs.txt` | `dir<TAB>struct<TAB>source file` | the wire-struct registry: every struct that is (or was) a JSON wire type |
| `golden_wire_fields.txt` | `dir<TAB>struct<TAB>field<TAB>key<TAB>base revision` | the pinned wire key of every field of every registered struct |
| `golden_wire_exceptions.txt` | `dir<TAB>struct<TAB>reason` | structs that expose json keys but are deliberately not pinned |
| `tag_allowlist.txt` | `path` or `path:line` | the permitted `yaml:`/`toml:` tag locations |
| `no_config_file_allowlist.txt` | `path` or `path:line` | the permitted mentions of a retired config file name |

`golden_wire_fields.txt` is generated, never hand-edited.

`tag_allowlist.txt` and `no_config_file_allowlist.txt` are two different
concepts and must not be conflated: the first permits `yaml:`/`toml:` struct
**tags**, the second permits mentions of the retired config **file names**.


## How the pinned keys are derived

The keys come from **immutable git history**, never from the working tree,
so regenerating the file can never absorb an unintended rename:

* a struct that existed at the baseline release (`v0.7.2`, the last release
  before the tags were stripped) is pinned at that tag;
* a struct created after the baseline release is pinned at the commit that
  introduced it;
* a struct that only became a wire type after the baseline release is
  pinned at the commit that established its wire contract. Such a struct
  existed at the baseline with untagged fields (or with none at all), and
  the commit that gave it its `json` tags never touches its `type X` line
  again — so the generator also looks for the commits that changed the
  struct's field lines, and pins the earliest one where every exported
  field carries a tag.

Regenerate with:

```sh
make tag-golden          # writes tools/tagcheck/golden_wire_fields.txt
make tag-golden-check    # fails when the committed file drifted from the history
```

Both need the full history and the baseline tag:

```sh
git fetch --tags --unshallow
```

The generator (`tools/tagcheck/cmd/taggolden`) reads the registry and the
history; it never reads the working tree for the pinned keys. The base
revision of every struct is recorded in the generated file, so the
derivation stays auditable.

## Why the key names are pinned

The rule set follows the compatibility contracts other ecosystems enforce:

* **buf** splits its breaking-change rules into `FILE`, `PACKAGE`, `WIRE`
  and `WIRE_JSON`; `WIRE_JSON` "detects changes that break wire (binary) or
  JSON encoding" and is "the recommended minimum level". Its
  `FIELD_SAME_JSON_NAME` rule "checks that the `json_name` field option
  doesn't change, which breaks JSON compatibility", and
  `FIELD_NO_DELETE_UNLESS_NAME_RESERVED` is "the JSON equivalent of
  reserving the number, since JSON uses field names instead of numbers".
  buf compares the current schema against "a past version", which can be a
  git repository — the same baseline model this guard uses.
  <https://buf.build/docs/breaking/> and <https://buf.build/docs/breaking/rules/>
* **Protocol Buffers** maps field names to JSON keys ("Message field names
  are mapped to lowerCamelCase to be used as JSON object keys. If the
  `json_name` field option is specified, the specified value will be used as
  the key instead"), and warns that the format "puts field and enum value
  names into encoded messages which makes it much harder to change those
  names later. Removing fields is a breaking change that will trigger a
  parsing error."
  <https://protobuf.dev/programming-guides/json/>
* **Kubernetes** requires that "once an API element has been added to an API
  group at a particular version, it can not be removed from that version or
  have its behavior significantly changed", and that objects "must be able to
  round-trip between API versions in a given release without information
  loss". An individual field "must exist and function until" its version is
  removed.
  <https://kubernetes.io/docs/reference/using-api/deprecation-policy/>
* **`encoding/json`** defines the exact semantics the guard enforces: "The
  format string gives the name of the field, possibly followed by a
  comma-separated list of options. The name may be empty in order to specify
  options without overriding the default field name", and "if the field tag
  is `-`, the field is always omitted".
  <https://pkg.go.dev/encoding/json#Marshal>
* **The Uber Go style guide** states the contract this guard protects: "The
  serialized form of the structure is a contract between different systems.
  Changes to the structure of the serialized form--including field
  names--break this contract."
  <https://github.com/uber-go/guide/blob/master/style.md#use-field-tags-in-marshaled-structs>
* **Schemathesis** shows the runtime counterpart of this guard: contract
  testing catches "schema violations where your API returns different data
  than documented", and supports a baseline mode that fails CI only on new
  violations. <https://github.com/schemathesis/schemathesis>

## The retired config-file rule (`no-config-file`)

v0.8.0 made the configuration env-only and removed the
`zever.json`/`zever.yaml`/`zever.yml`/`zever.toml` project files for good.
The framework does not know those names any more: a stale file is ignored
silently, there is no probe and no warning. The `no-config-file` rule makes
that break permanent by failing the gate when a **non-test, non-generated Go
file names one of the retired formats**.

### The contract

> No non-test Go file may name a retired project config file.

A "name" is any of:

* a **string literal** that contains one — a path (`os.ReadFile("zever.yaml")`,
  `filepath.Join(dir, "zever.yaml")`), a message, a constant, a struct tag, an
  import path;
* a **constant string expression** that evaluates to one — a concatenation of
  literals (`"zever" + ".yaml"`), a `fmt.Sprintf` with a literal format and
  literal arguments (`fmt.Sprintf("zever.%s", "yaml")`), a `strings.Join` of a
  literal slice (`strings.Join([]string{"zever", ".yml"}, "")`);
* a **comment** — prose about a format that no longer exists is a leftover
  of the removed formats, and it is what misleads the next reader.

The match is **case-insensitive** (`Zever.YAML` is a hit) and sits on a **name
boundary**: the characters around it must not extend the name, so
`zever.schema.json` (a different file) and `foozever.yaml` are not hits.

One violation is reported per line; the line carries the first retired name
found on it. The reported line is the line the name sits on, even inside a
multi-line raw string (a usage text, for example).

### Why a syntactic scan, not an import or binding analysis

The formats are gone, so there is no import path and no symbol to analyze:
the only trace of them is a *string*. An import/binding analysis (the shape
`depguard` or ESLint's `no-restricted-imports` use) would find nothing —
there is no package left to import. The rule therefore scans the string
values themselves, which is the shape ESLint's `no-restricted-imports`
`patterns`/`regex` options and semgrep's `pattern-regex`/`metavariable-regex`
operators use for exactly this problem: a name that must not come back.

The scan is deliberately cheap and total: it parses the file with `go/parser`
(already done for the tag rules) and evaluates the constant string
expressions of the file. It does not type-check, so it cannot be fooled by a
runtime value — but a runtime value cannot name a file either: the retired
names are constants.

### The guard is subject to its own rule

The guard builds the retired names at runtime (`legacyConfigPrefix` plus the
`json`/`yaml`/`yml`/`toml` extensions), so its own source never spells one:
there is no exemption entry, and the guard fails on a mention in its own
files exactly like in any other file. The names are listed in this README,
which is not a Go file and is not scanned.

### Exceptions

`no_config_file_allowlist.txt` holds the exceptions: one entry per line,
either a bare repo-relative path (the whole file) or `path:line` (one line).
The list starts empty. An entry is justified only when the mention is
load-bearing and unavoidable — a code generator that has to write the name
into a legacy project, or a migration tool that must still clean one up. The
framework itself must never name a retired config file.

### Why this shape

The rule follows the compatibility contracts other ecosystems enforce:

* **buf** splits its breaking-change rules into `FILE`, `PACKAGE`, `WIRE`
  and `WIRE_JSON`, and pins the current tree against "a past version",
  which can be a git repository — the same baseline model this guard uses.
  Its `FILE_NO_DELETE` rule "checks that no files are deleted from a given
  file": a removed format that comes back is the failure this rule prevents.
  <https://buf.build/docs/breaking/> and <https://buf.build/docs/breaking/rules/>
* **Kubernetes** requires that "once an API element has been added to an API
  group at a particular version, it can not be removed from that version or
  have its behavior significantly changed", and that the rules "are enforced
  between official releases, not between arbitrary commits" — a policy that
  only exists as prose is not a policy, so it is enforced by a gate.
  <https://kubernetes.io/docs/reference/using-api/deprecation-policy/>
* **ESLint** ships `no-restricted-imports` for the same job ("You can
  specify the name of a module to restrict from being imported"), with
  `paths`, `patterns` and a `regex` option, case-insensitive by default
  (`caseSensitive: false`) — the shape of this rule: a name, a pattern, a
  case-insensitive match, and an explicit allowlist.
  <https://eslint.org/docs/latest/rules/no-restricted-imports>
* **semgrep** encodes "this symbol/format must not come back" as a
  `pattern-regex` or `metavariable-regex` rule over string literals, and
  excludes the test files through a `paths` filter — the same two decisions
  this rule makes (scan the literals, skip `_test.go`).
  <https://semgrep.dev/docs/writing-rules/rule-syntax>
* **`golangci-lint`'s `depguard`** is the in-repo precedent for a syntactic
  ban with an exception list: this repo already denies `github.com/pkg/errors`
  and `io/ioutil` that way (see `.golangci.yml`).
  <https://golangci-lint.run/docs/linters/configuration/#depguard>

## Registering a new wire struct

A struct that exposes json keys — an exported field with a non-empty,
non-`-` json name — is a wire struct. It must be registered, so that its
keys get pinned:

```sh
# 1. list the wire structs of the tree that are not registered yet
go -C tools/tagcheck run ./cmd/taggolden -root . -list-unregistered

# 2. add a line to golden_wire_structs.txt, then pin its keys
make tag-golden
```

A struct that mixes json-tagged fields with a Go-API-only field (a logger,
a store handle) cannot be registered: the registry pins every exported
field of a registered struct, and an untagged field has no wire key to
pin. Such a struct goes into `golden_wire_exceptions.txt` with a
justification. The guard rejects an exception that names a struct which
does not exist, that is registered already, or that is fully json-tagged.

## Why "exposes a json key" is the registration signal

The `unregistered-wire` rule asks whether a struct reaches a JSON
marshaller. A precise answer needs type-flow analysis: `musttag` resolves
the argument type of `json.Marshal` with `go/types` and checks the tags of
the struct it finds (<https://github.com/go-simpler/musttag>). That
approach only sees the *static* type at the call site: a value held in an
`any`, returned by a function, or stored in a `map[string]any` (as
`serve/doctor.go` does with its doctor report) hides the struct, so the
analysis is both incomplete and expensive — it needs a type-checked
program, not just a parsed file.

The guard therefore uses a precise, purely syntactic signal instead: a
struct that exposes at least one json key is a wire struct and must be
registered. The criterion is the same one the registry was originally
derived with, it has no false positives on the current tree, and the
exceptions file keeps the few deliberate hybrids (a config `Options`
struct that mixes json tags with a Go-API-only handle) visible and
reviewable.
