package main

import (
	"strings"
	"testing"
)

// parserViolations returns the no-config-parser violations of a fixture
// scan.
func parserViolations(t *testing.T, files map[string]string, parserAllowlist string) []violation {
	t.Helper()

	vs := scanFixtureWithParser(t, files, "", "", "", "", "", parserAllowlist)

	out := make([]violation, 0, len(vs))

	for _, v := range vs {
		if v.rule == "no-config-parser" {
			out = append(out, v)
		}
	}

	return out
}

func TestScanNoConfigParserImport(t *testing.T) {
	t.Parallel()

	vs := parserViolations(t, map[string]string{
		"pkg/parser.go": "yaml_import.txt",
	}, "")

	if len(vs) != 1 {
		t.Fatalf("got %v, want exactly one no-config-parser violation", vs)
	}

	if !strings.Contains(vs[0].msg, "go.yaml.in/yaml/v3") {
		t.Errorf("message %q does not name the import", vs[0].msg)
	}

	if vs[0].path != "pkg/parser.go" {
		t.Errorf("path %q is not the scanned file", vs[0].path)
	}
}

func TestScanNoConfigParserTomlImport(t *testing.T) {
	t.Parallel()

	vs := parserViolations(t, map[string]string{
		"pkg/parser.go": "toml_import.txt",
	}, "")

	if len(vs) != 1 {
		t.Fatalf("got %v, want exactly one no-config-parser violation", vs)
	}

	if !strings.Contains(vs[0].msg, "BurntSushi/toml") {
		t.Errorf("message %q does not name the import", vs[0].msg)
	}
}

func TestScanNoConfigParserSkipsTestFiles(t *testing.T) {
	t.Parallel()

	vs := parserViolations(t, map[string]string{
		"pkg/parser_test.go": "yaml_import.txt",
	}, "")

	if len(vs) != 0 {
		t.Fatalf("test file produced violations: %v", vs)
	}
}

func TestScanNoConfigParserAllowlistedFile(t *testing.T) {
	t.Parallel()

	vs := parserViolations(t, map[string]string{
		"pkg/parser.go": "yaml_import.txt",
	}, "pkg/parser.go\n")

	if len(vs) != 0 {
		t.Fatalf("allowlisted file produced violations: %v", vs)
	}
}

func TestScanNoConfigParserAllowlistedLine(t *testing.T) {
	t.Parallel()

	vs := parserViolations(t, map[string]string{
		"pkg/parser.go": "yaml_import.txt",
	}, "pkg/parser.go:4\n")

	if len(vs) != 0 {
		t.Fatalf("allowlisted line produced violations: %v", vs)
	}
}

func TestScanNoConfigParserAllowlistOtherLine(t *testing.T) {
	t.Parallel()

	vs := parserViolations(t, map[string]string{
		"pkg/parser.go": "yaml_import.txt",
	}, "pkg/parser.go:3\n")

	if len(vs) != 1 {
		t.Fatalf("got %v, want exactly one no-config-parser violation", vs)
	}
}

func TestLoadNoConfigParserAllowlistMissingFile(t *testing.T) {
	t.Parallel()

	a, err := loadNoConfigParserAllowlist(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	if len(a) != 0 {
		t.Fatalf("got %v, want an empty allowlist", a)
	}
}

func TestConfigParserImport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		ok   bool
	}{
		{name: "yaml v3", path: "go.yaml.in/yaml/v3", ok: true},
		{name: "yaml v4", path: "go.yaml.in/yaml/v4", ok: true},
		{name: "yaml v2", path: "gopkg.in/yaml.v2", ok: true},
		{name: "yaml v3 legacy", path: "github.com/gopkg.in/yaml.v3", ok: true},
		{name: "burnt sushi toml", path: "github.com/BurntSushi/toml", ok: true},
		{name: "pelletier toml v2", path: "github.com/pelletier/go-toml/v2", ok: true},
		{name: "unrelated package", path: "github.com/zenta-dev/zever/config", ok: false},
		{name: "unrelated yaml name", path: "example.com/yamlx", ok: false},
		{name: "unrelated toml name", path: "example.com/tomlx", ok: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, ok := configParserImport(test.path)
			if ok != test.ok {
				t.Fatalf("import %q matched=%v, want %v", test.path, ok, test.ok)
			}
		})
	}
}
