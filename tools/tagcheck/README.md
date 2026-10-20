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
| `golden-removed` | a registered wire struct that disappeared from the tree |

Files that are skipped: `_test.go` files, generated files (a `DO NOT EDIT`
marker in a leading comment), and the `.git`, `.worktrees`, `.claude`,
`vendor`, `node_modules` and `generated` directories.

## Data files

| file | shape | meaning |
|---|---|---|
| `golden_wire_structs.txt` | `dir<TAB>struct<TAB>source file` | the wire-struct registry: every struct that is (or was) a JSON wire type |
| `golden_wire_fields.txt` | `dir<TAB>struct<TAB>field<TAB>key<TAB>base revision` | the pinned wire key of every field of every registered struct |
| `tag_allowlist.txt` | `path` or `path:line` | the permitted `yaml:`/`toml:` tag locations |

`golden_wire_fields.txt` is generated, never hand-edited.

## How the pinned keys are derived

The keys come from **immutable git history**, never from the working tree,
so regenerating the file can never absorb an unintended rename:

* a struct that existed at the baseline release (`v0.7.2`, the last release
  before the tags were stripped) is pinned at that tag;
* a struct created after the baseline release is pinned at the commit that
  introduced it.

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
