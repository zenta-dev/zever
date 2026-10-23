package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/tools/tagcheck/internal/wire"
)

// BenchmarkLoadGolden measures the golden structs loading hot path.
func BenchmarkLoadGolden(b *testing.B) {
	content := `config	Options	options.go
config	Env	env.go
container	Config	config.go
orm	Table	table.go
orm	Column	column.go
`

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = loadGoldenFromString(content)
	}
}

// loadGoldenFromString parses the golden list from a string.
func loadGoldenFromString(content string) (goldenStructs, error) {
	r := strings.NewReader(content)
	g := goldenStructs{}

	parseErr := eachDataLine(r, "test", func(line string) error {
		parts := strings.Split(line, "\t")
		if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
			return nil
		}

		names := g[parts[0]]
		if names == nil {
			names = map[string]string{}
			g[parts[0]] = names
		}

		if len(parts) >= 3 {
			names[parts[1]] = parts[2]
		} else {
			names[parts[1]] = ""
		}

		return nil
	})
	return g, parseErr
}

// BenchmarkLoadGoldenFields measures the golden fields loading hot path.
func BenchmarkLoadGoldenFields(b *testing.B) {
	content := `config	Options	MaxConns	max_conns
config	Options	Timeout	timeout
container	Config	DB	db
orm	Table	Name	name
orm	Column	Name	name
orm	Column	Type	type
`

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = loadGoldenFieldsFromString(content)
	}
}

// loadGoldenFieldsFromString parses the golden fields from a string.
func loadGoldenFieldsFromString(content string) (goldenFields, error) {
	r := strings.NewReader(content)
	g := goldenFields{}

	parseErr := eachDataLine(r, "test", func(line string) error {
		parts := strings.Split(line, "\t")
		if len(parts) < 4 || parts[0] == "" || parts[1] == "" || parts[2] == "" || parts[3] == "" {
			return nil
		}

		names := g[parts[0]]
		if names == nil {
			names = map[string]map[string]string{}
			g[parts[0]] = names
		}

		fields := names[parts[1]]
		if fields == nil {
			fields = map[string]string{}
			names[parts[1]] = fields
		}

		fields[parts[2]] = parts[3]

		return nil
	})
	return g, parseErr
}

// BenchmarkLoadExceptions measures the exceptions loading hot path.
func BenchmarkLoadExceptions(b *testing.B) {
	content := `config	SomeStruct	justification
container	AnotherStruct	another reason
orm	YetAnotherStruct	yet another reason
`

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = loadExceptionsFromString(content)
	}
}

// loadExceptionsFromString parses the exceptions from a string.
func loadExceptionsFromString(content string) (exceptions, error) {
	r := strings.NewReader(content)
	e := exceptions{}

	parseErr := eachDataLine(r, "test", func(line string) error {
		parts := strings.Split(line, "\t")
		if len(parts) < 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
			return nil
		}

		names := e[parts[0]]
		if names == nil {
			names = map[string]string{}
			e[parts[0]] = names
		}

		names[parts[1]] = parts[2]

		return nil
	})
	return e, parseErr
}

// BenchmarkLoadAllowlist measures the allowlist loading hot path.
func BenchmarkLoadAllowlist(b *testing.B) {
	content := `config/options.go
config/env.go:15
container/config.go
orm/table.go
`

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = loadAllowlistFromString(content)
	}
}

// loadAllowlistFromString parses the allowlist from a string.
func loadAllowlistFromString(content string) (allowlist, error) {
	r := strings.NewReader(content)
	a := allowlist{}

	parseErr := eachDataLine(r, "test", func(line string) error {
		a[strings.TrimSpace(line)] = true
		return nil
	})
	return a, parseErr
}

// BenchmarkEachDataLine measures the line-by-line parsing hot path.
func BenchmarkEachDataLine(b *testing.B) {
	content := `line1
# comment
line2
line3

line4
# another comment
line5
line6
line7
line8
line9
line10
`

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		r := strings.NewReader(content)
		_ = eachDataLine(r, "test", func(line string) error {
			_ = line
			return nil
		})
	}
}

// BenchmarkSkipDir measures the directory skip logic.
func BenchmarkSkipDir(b *testing.B) {
	dirs := []string{
		".git", ".worktrees", ".claude", "vendor", "node_modules",
		"generated", "testdata", "config", "container", "orm",
	}
	i := 0

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = skipDir(dirs[i%len(dirs)])
		i++
	}
}

// BenchmarkCheckLegacyConfigNames measures the legacy config file name detection.
func BenchmarkCheckLegacyConfigNames(b *testing.B) {
	// We can't easily test without a full AST, so test the string matching logic
	patterns := []string{
		"zever.yaml", "zever.yml", "zever.json", "zever.toml",
		"zever.cfg", "zever.conf", "zever.ini", "zever.properties",
		"config.yaml", "config.yml", "config.json",
	}
	i := 0

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		s := patterns[i%len(patterns)]
		_ = strings.Contains(s, "zever.")
		i++
	}
}

// BenchmarkCollectFields measures the struct field collection hot path.
func BenchmarkCollectFields(b *testing.B) {
	// Simulated field data - we test the logic without AST
	fieldCount := 20

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		fields := make([]fieldInfo, fieldCount)
		for i := 0; i < fieldCount; i++ {
			fields[i] = fieldInfo{
				Name:   "Field" + string(rune('A'+i)),
				Tag:    "`json:\"field_\" + string(rune('a'+i)) + \"\"`",
				HasTag: true,
				line:   i + 10,
			}
		}
		_ = fields
	}
}

// BenchmarkCheckStruct_PinnedKey measures the pinned key validation.
func BenchmarkCheckStruct_PinnedKey(b *testing.B) {
	fields := make([]wire.Field, 10)
	for i := range fields {
		fields[i] = wire.Field{
			Name:   "Field" + string(rune('A'+i)),
			Tag:    "`json:\"field_\" + string(rune('a'+i)) + \"\"`",
			HasTag: true,
		}
	}
	pinned := make(map[string]string)
	for i := range fields {
		pinned[fields[i].Name] = "field_" + string(rune('a'+i))
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		for _, field := range fields {
			want := pinned[field.Name]
			if field.State() == wire.StateNamed {
				got, _ := field.Key()
				if got != want {
					// violation
				}
			}
		}
	}
}

// BenchmarkCheckUnregistered measures the unregistered struct check.
func BenchmarkCheckUnregistered(b *testing.B) {
	fields := make([]wire.Field, 10)
	for i := range fields {
		fields[i] = wire.Field{
			Name:   "Field" + string(rune('A'+i)),
			Tag:    "`json:\"field_\" + string(rune('a'+i)) + \"\"`",
			HasTag: true,
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		exposesKey := false
		for _, field := range fields {
			if field.State() == wire.StateNamed {
				exposesKey = true
				break
			}
		}
		_ = exposesKey
	}
}

// BenchmarkCheckGoldenKey measures the golden key comparison.
func BenchmarkCheckGoldenKey(b *testing.B) {
	field := wire.Field{
		Name:   "FieldName",
		Tag:    `json:"field_name"`,
		HasTag: true,
	}
	want := "field_name"

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if field.State() == wire.StateNamed {
			got, _ := field.Key()
			if got != want {
				// violation
			}
		}
	}
}

// BenchmarkCheckPinnedKeys measures the pinned key completeness check.
func BenchmarkCheckPinnedKeys(b *testing.B) {
	pinned := make(map[string]string)
	for i := 0; i < 10; i++ {
		pinned["Field"+string(rune('A'+i))] = "field_" + string(rune('a'+i))
	}
	fields := make([]wire.Field, 10)
	for i := range fields {
		fields[i] = wire.Field{
			Name:   "Field" + string(rune('A'+i)),
			Tag:    "`json:\"field_\" + string(rune('a'+i)) + \"\"`",
			HasTag: true,
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		for goFieldName := range pinned {
			found := false
			for _, field := range fields {
				if field.Name == goFieldName {
					found = true
					break
				}
			}
			if !found {
				// violation - golden-missing
			}
		}
	}
}

// BenchmarkWireFieldState measures the wire.Field.State() hot path.
func BenchmarkWireFieldState(b *testing.B) {
	fields := []wire.Field{
		{Name: "NoTag", HasTag: false},
		{Name: "EmptyTag", HasTag: true, Tag: ""},
		{Name: "DashTag", HasTag: true, Tag: `json:"-"`},
		{Name: "NamedTag", HasTag: true, Tag: `json:"named"`},
		{Name: "OtherTag", HasTag: true, Tag: `yaml:"other"`},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		for _, f := range fields {
			_ = f.State()
		}
	}
}

// BenchmarkWireFieldKey measures the wire.Field.Key() hot path.
func BenchmarkWireFieldKey(b *testing.B) {
	fields := []wire.Field{
		{Name: "NamedTag", HasTag: true, Tag: `json:"field_name"`},
		{Name: "OtherTag", HasTag: true, Tag: `json:"other"`},
		{Name: "MultipleTags", HasTag: true, Tag: `json:"multi" yaml:"other"`},
		{Name: "NoJSON", HasTag: true, Tag: `yaml:"only"`},
		{Name: "NoTag", HasTag: false},
		{Name: "DashTag", HasTag: true, Tag: `json:"-"`},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		for _, f := range fields {
			_, _ = f.Key()
		}
	}
}

// BenchmarkStructTagLookup measures reflect.StructTag.Lookup hot path.
func BenchmarkStructTagLookup(b *testing.B) {
	tags := []string{
		`json:"field_name"`,
		`json:"other" yaml:"yaml_field"`,
		`yaml:"only"`,
		`toml:"toml_field" json:"json_field"`,
		`json:"-"`,
		``,
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		for _, tag := range tags {
			st := reflect.StructTag(tag)
			_, _ = st.Lookup("json")
			_, _ = st.Lookup("yaml")
			_, _ = st.Lookup("toml")
		}
	}
}

// BenchmarkCheckConfigParserImports measures the config parser import detection.
func BenchmarkCheckConfigParserImports(b *testing.B) {
	importPaths := []string{
		"gopkg.in/yaml.v3",
		"github.com/BurntSushi/toml",
		"github.com/knadh/koanf/parsers/yaml",
		"github.com/knadh/koanf/parsers/toml",
		"github.com/mitchellh/mapstructure",
		"fmt",
		"github.com/zenta-dev/zever/config",
	}
	i := 0

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		path := importPaths[i%len(importPaths)]
		_ = strings.HasPrefix(path, "gopkg.in/yaml")
		_ = strings.HasPrefix(path, "github.com/BurntSushi/toml")
		_ = strings.HasPrefix(path, "github.com/knadh/koanf/parsers")
		i++
	}
}

// BenchmarkStringSplit measures strings.Split for tab-separated parsing.
func BenchmarkStringSplit(b *testing.B) {
	lines := []string{
		"config\tOptions\toptions.go",
		"config\tEnv\t\tenv.go",
		"orm\tTable\t\ttable.go\tv0.7.2:table.go",
		"adapters/cache/redis\tRedisCache\tredis_cache.go",
	}
	i := 0

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = strings.Split(lines[i%len(lines)], "\t")
		i++
	}
}