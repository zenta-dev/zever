package main

import (
	"go/ast"
	"go/parser"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// parseExpr parses a Go expression, failing the test when it cannot.
func parseExpr(t *testing.T, src string) ast.Expr {
	t.Helper()

	expr, err := parser.ParseExpr(src)
	if err != nil {
		t.Fatal(err)
	}

	return expr
}

// legacyViolations returns the no-config-file violations of a fixture scan.
func legacyViolations(t *testing.T, files map[string]string, noConfigAllowlist string) []violation {
	t.Helper()

	vs := scanFixtureWith(t, files, "", "", "", "", noConfigAllowlist)

	out := make([]violation, 0, len(vs))

	for _, v := range vs {
		if v.rule == "no-config-file" {
			out = append(out, v)
		}
	}

	return out
}

func TestScanNoConfigFileInPath(t *testing.T) {
	t.Parallel()

	vs := legacyViolations(t, map[string]string{
		"pkg/legacy.go": "legacy_path.txt",
	}, "")

	if len(vs) != 1 {
		t.Fatalf("got %v, want exactly one no-config-file violation", vs)
	}

	if !strings.Contains(vs[0].msg, "zever.yaml") {
		t.Errorf("message %q does not name the retired file", vs[0].msg)
	}

	if vs[0].path != "pkg/legacy.go" {
		t.Errorf("path %q is not the scanned file", vs[0].path)
	}
}

func TestScanNoConfigFileInMessage(t *testing.T) {
	t.Parallel()

	vs := legacyViolations(t, map[string]string{
		"pkg/legacy.go": "legacy_message.txt",
	}, "")

	if len(vs) != 1 {
		t.Fatalf("got %v, want exactly one no-config-file violation", vs)
	}

	if !strings.Contains(vs[0].msg, "zever.json") {
		t.Errorf("message %q does not name the retired file", vs[0].msg)
	}
}

func TestScanNoConfigFileConcatenated(t *testing.T) {
	t.Parallel()

	vs := legacyViolations(t, map[string]string{
		"pkg/legacy.go": "legacy_concat.txt",
	}, "")

	if len(vs) != 1 {
		t.Fatalf("got %v, want exactly one no-config-file violation", vs)
	}

	if !strings.Contains(vs[0].msg, "zever.yaml") {
		t.Errorf("message %q does not name the retired file", vs[0].msg)
	}
}

func TestScanNoConfigFileFormatted(t *testing.T) {
	t.Parallel()

	vs := legacyViolations(t, map[string]string{
		"pkg/legacy.go": "legacy_format.txt",
	}, "")

	if len(vs) != 1 {
		t.Fatalf("got %v, want exactly one no-config-file violation", vs)
	}

	if !strings.Contains(vs[0].msg, "zever.yaml") {
		t.Errorf("message %q does not name the retired file", vs[0].msg)
	}
}

func TestScanNoConfigFileJoined(t *testing.T) {
	t.Parallel()

	vs := legacyViolations(t, map[string]string{
		"pkg/legacy.go": "legacy_join.txt",
	}, "")

	if len(vs) != 1 {
		t.Fatalf("got %v, want exactly one no-config-file violation", vs)
	}

	if !strings.Contains(vs[0].msg, "zever.yml") {
		t.Errorf("message %q does not name the retired file", vs[0].msg)
	}
}

func TestScanNoConfigFileCaseInsensitive(t *testing.T) {
	t.Parallel()

	vs := legacyViolations(t, map[string]string{
		"pkg/legacy.go": "legacy_case.txt",
	}, "")

	if len(vs) != 2 {
		t.Fatalf("got %v, want one violation for the literal and one for the comment", vs)
	}

	for _, v := range vs {
		if !strings.Contains(strings.ToLower(v.msg), "zever.yaml") {
			t.Errorf("message %q does not name the retired file", v.msg)
		}
	}
}

func TestScanNoConfigFileInMultiLineLiteral(t *testing.T) {
	t.Parallel()

	vs := legacyViolations(t, map[string]string{
		"pkg/legacy.go": "legacy_rawstring.txt",
	}, "")

	if len(vs) != 1 {
		t.Fatalf("got %v, want exactly one no-config-file violation", vs)
	}

	if vs[0].line != 5 {
		t.Errorf("violation reported at line %d, want the line the name sits on (5)", vs[0].line)
	}
}

func TestScanNoConfigFileClean(t *testing.T) {
	t.Parallel()

	vs := legacyViolations(t, map[string]string{
		"pkg/clean.go": "clean.txt",
	}, "")

	if len(vs) != 0 {
		t.Fatalf("clean file produced violations: %v", vs)
	}
}

func TestScanNoConfigFileNewExtension(t *testing.T) {
	t.Parallel()

	vs := legacyViolations(t, map[string]string{
		"pkg/legacy.go": "legacy_ini.txt",
	}, "")

	if len(vs) != 1 {
		t.Fatalf("got %v, want exactly one no-config-file violation", vs)
	}

	if !strings.Contains(vs[0].msg, "zever.ini") {
		t.Errorf("message %q does not name the retired file", vs[0].msg)
	}
}

func TestLegacyConfigRuleNames(t *testing.T) {
	t.Parallel()

	rule := newLegacyConfigRule()

	wantExts := []string{"json", "yaml", "yml", "toml", "cfg", "conf", "ini", "properties"}

	if len(rule.names) != len(wantExts) {
		t.Fatalf("rule names %q, want one per retired extension %q", rule.names, wantExts)
	}

	for i, ext := range wantExts {
		if want := legacyConfigPrefix + ext; rule.names[i] != want {
			t.Errorf("rule names[%d] = %q, want %q", i, rule.names[i], want)
		}
	}
}

func TestScanNoConfigFileIgnoresOtherNames(t *testing.T) {
	t.Parallel()

	vs := legacyViolations(t, map[string]string{
		"pkg/other.go": "other_names.txt",
	}, "")

	if len(vs) != 0 {
		t.Fatalf("other file names produced violations: %v", vs)
	}
}

func TestScanNoConfigFileSkipsTestFiles(t *testing.T) {
	t.Parallel()

	vs := legacyViolations(t, map[string]string{
		"pkg/legacy_test.go": "legacy_path.txt",
	}, "")

	if len(vs) != 0 {
		t.Fatalf("test file produced violations: %v", vs)
	}
}

func TestScanNoConfigFileAppliesToGeneratedFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	content, err := os.ReadFile(filepath.Join("testdata", "legacy_path.txt"))
	if err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(root, "pkg", "generated.go"), "// Code generated by hand; DO NOT EDIT.\n\n"+string(content))

	toolDir := filepath.Join(root, "tools", "tagcheck")

	writeFile(t, filepath.Join(toolDir, "golden_wire_structs.txt"), "")
	writeFile(t, filepath.Join(toolDir, "golden_wire_fields.txt"), "")
	writeFile(t, filepath.Join(toolDir, "golden_wire_exceptions.txt"), "")
	writeFile(t, filepath.Join(toolDir, "tag_allowlist.txt"), "")

	vs, err := scan(root, rules{})
	if err != nil {
		t.Fatal(err)
	}

	if len(vs) != 1 || vs[0].rule != "no-config-file" {
		t.Fatalf("got %v, want exactly one no-config-file violation", vs)
	}

	if !strings.Contains(vs[0].msg, "zever.yaml") {
		t.Errorf("message %q does not name the retired file", vs[0].msg)
	}

	if vs[0].path != "pkg/generated.go" {
		t.Errorf("path %q is not the scanned generated file", vs[0].path)
	}
}

func TestScanGeneratedFileOtherRulesStillSkipped(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	content, err := os.ReadFile(filepath.Join("testdata", "empty_tag.txt"))
	if err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(root, "pkg", "generated.go"), "// Code generated by hand; DO NOT EDIT.\n\n"+string(content))

	toolDir := filepath.Join(root, "tools", "tagcheck")

	writeFile(t, filepath.Join(toolDir, "golden_wire_structs.txt"), "")
	writeFile(t, filepath.Join(toolDir, "golden_wire_fields.txt"), "")
	writeFile(t, filepath.Join(toolDir, "golden_wire_exceptions.txt"), "")
	writeFile(t, filepath.Join(toolDir, "tag_allowlist.txt"), "")

	vs, err := scan(root, rules{})
	if err != nil {
		t.Fatal(err)
	}

	if len(vs) != 0 {
		t.Fatalf("generated file with only an unrelated tag problem produced violations: %v", vs)
	}
}

func TestScanNoConfigFileAllowlistedFile(t *testing.T) {
	t.Parallel()

	vs := legacyViolations(t, map[string]string{
		"pkg/legacy.go": "legacy_path.txt",
	}, "pkg/legacy.go\n")

	if len(vs) != 0 {
		t.Fatalf("allowlisted file produced violations: %v", vs)
	}
}

func TestScanNoConfigFileAllowlistedLine(t *testing.T) {
	t.Parallel()

	vs := legacyViolations(t, map[string]string{
		"pkg/legacy.go": "legacy_path.txt",
	}, "pkg/legacy.go:7\n")

	if len(vs) != 0 {
		t.Fatalf("allowlisted line produced violations: %v", vs)
	}
}

func TestScanNoConfigFileAllowlistOtherLine(t *testing.T) {
	t.Parallel()

	vs := legacyViolations(t, map[string]string{
		"pkg/legacy.go": "legacy_path.txt",
	}, "pkg/legacy.go:3\n")

	if len(vs) != 1 {
		t.Fatalf("got %v, want exactly one no-config-file violation", vs)
	}
}

func TestLoadNoConfigAllowlistMissingFile(t *testing.T) {
	t.Parallel()

	a, err := loadNoConfigAllowlist(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	if len(a) != 0 {
		t.Fatalf("got %v, want an empty allowlist", a)
	}
}

func TestLegacyConfigRuleMatch(t *testing.T) {
	t.Parallel()

	rule := newLegacyConfigRule()

	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "exact", value: "zever.yaml", want: "zever.yaml"},
		{name: "path", value: "dir/zever.yaml", want: "zever.yaml"},
		{name: "upper case", value: "Zever.YAML", want: "Zever.YAML"},
		{name: "json", value: "zever.json", want: "zever.json"},
		{name: "toml", value: "zever.toml", want: "zever.toml"},
		{name: "cfg", value: "zever.cfg", want: "zever.cfg"},
		{name: "conf", value: "zever.conf", want: "zever.conf"},
		{name: "ini", value: "zever.ini", want: "zever.ini"},
		{name: "properties", value: "zever.properties", want: "zever.properties"},
		{name: "sentence", value: "the zever.ini file is ignored", want: "zever.ini"},
		{name: "upper case", value: "Zever.CONF", want: "Zever.CONF"},
		{name: "longer name", value: "zever.yaml.bak", want: "zever.yaml"},
		{name: "other schema", value: "zever.schema.json", want: ""},
		{name: "other prefix", value: "foozever.yaml", want: ""},
		{name: "other package", value: "zever.annotations.v1.auth", want: ""},
		{name: "versioned api package", value: "zever.v1", want: ""},
		{name: "schema helper", value: "zever.schema", want: ""},
		{name: "shop helper", value: "zever.shop", want: ""},
		{name: "new helper", value: "zever.new", want: ""},
		{name: "unrelated", value: "yaml", want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, ok := rule.match(test.value)
			if test.want == "" {
				if ok {
					t.Fatalf("value %q matched %q, want no match", test.value, got)
				}

				return
			}

			if !ok {
				t.Fatalf("value %q did not match, want %q", test.value, test.want)
			}

			if got != test.want {
				t.Fatalf("value %q matched %q, want %q", test.value, got, test.want)
			}
		})
	}
}

func TestConstString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		expr string
		want string
		ok   bool
	}{
		{name: "literal", expr: `"zever.yaml"`, want: "zever.yaml", ok: true},
		{name: "concat", expr: `"zever" + ".yaml"`, want: "zever.yaml", ok: true},
		{name: "paren", expr: `("zever.yaml")`, want: "zever.yaml", ok: true},
		{name: "sprintf", expr: `fmt.Sprintf("zever.%s", "yaml")`, want: "zever.yaml", ok: true},
		{name: "join", expr: `strings.Join([]string{"zever", ".yaml"}, "")`, want: "zever.yaml", ok: true},
		{name: "runtime value", expr: `fmt.Sprintf("zever.%s", ext)`, ok: false},
		{name: "runtime concat", expr: `"zever." + ext`, ok: false},
		{name: "call", expr: `os.ReadFile("zever.yaml")`, ok: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			expr := parseExpr(t, test.expr)

			got, ok := constString(expr)
			if ok != test.ok {
				t.Fatalf("expression %q evaluated to (%q, %v), want ok=%v", test.expr, got, ok, test.ok)
			}

			if ok && got != test.want {
				t.Fatalf("expression %q evaluated to %q, want %q", test.expr, got, test.want)
			}
		})
	}
}
