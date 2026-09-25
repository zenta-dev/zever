package config

import _ "embed"

// SchemaJSON is the JSON Schema (draft 2020-12) describing the shape of
// zever.yaml/.yml/.json, embedded from schema/zever.schema.json so callers
// (for example `zever new`'s scaffolder) can ship it into a generated
// project without reading it off disk at runtime. See schema/README.md for
// the YAML/TOML equivalents and editor wiring.
//
//go:embed schema/zever.schema.json
var SchemaJSON []byte
