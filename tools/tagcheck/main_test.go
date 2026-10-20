package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFile copies fixture content (testdata/*.txt) to path, failing the
// test on any error.
func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// writeGo copies a testdata/*.txt fixture to a .go file under root.
func writeGo(t *testing.T, root, rel, fixture string) {
	t.Helper()

	content, err := os.ReadFile(filepath.Join("testdata", fixture))
	if err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(root, filepath.FromSlash(rel)), string(content))
}

// scanFixture builds a temp tree from fixtures and returns its violations.
func scanFixture(t *testing.T, files map[string]string, golden, allowlist string) []violation {
	t.Helper()

	root := t.TempDir()

	for rel, fixture := range files {
		if fixture == "" {
			writeFile(t, filepath.Join(root, filepath.FromSlash(rel)), "")
			continue
		}

		if strings.HasSuffix(rel, ".txt") {
			writeFile(t, filepath.Join(root, filepath.FromSlash(rel)), fixture)
			continue
		}

		writeGo(t, root, rel, fixture)
	}

	writeFile(t, filepath.Join(root, "tools", "tagcheck", "golden_wire_structs.txt"), golden)
	writeFile(t, filepath.Join(root, "tools", "tagcheck", "tag_allowlist.txt"), allowlist)

	g, err := loadGolden(filepath.Join(root, "tools", "tagcheck", "golden_wire_structs.txt"))
	if err != nil {
		t.Fatal(err)
	}

	a, err := loadAllowlist(filepath.Join(root, "tools", "tagcheck", "tag_allowlist.txt"))
	if err != nil {
		t.Fatal(err)
	}

	vs, err := scan(root, g, a)
	if err != nil {
		t.Fatal(err)
	}

	return vs
}

// rules returns the set of rule names in vs.
func rules(vs []violation) []string {
	out := make([]string, 0, len(vs))
	for _, v := range vs {
		out = append(out, v.rule)
	}

	return out
}

func TestScanCleanFile(t *testing.T) {
	t.Parallel()

	vs := scanFixture(t, map[string]string{
		"pkg/clean.go":                           "clean.txt",
		"tools/tagcheck/golden_wire_structs.txt": "",
		"tools/tagcheck/tag_allowlist.txt":       "",
	}, "", "")

	if len(vs) != 0 {
		t.Fatalf("clean file produced violations: %v", vs)
	}
}

func TestScanEmptyTag(t *testing.T) {
	t.Parallel()

	vs := scanFixture(t, map[string]string{
		"pkg/wire.go": "empty_tag.txt",
	}, "", "")

	if len(vs) != 1 || vs[0].rule != "empty-tag" {
		t.Fatalf("got %v, want exactly one empty-tag violation", vs)
	}

	if !strings.Contains(vs[0].msg, "empty struct tag") {
		t.Errorf("message %q does not explain the empty tag", vs[0].msg)
	}
}

func TestScanForbiddenYamlTag(t *testing.T) {
	t.Parallel()

	vs := scanFixture(t, map[string]string{
		"pkg/wire.go": "yaml_tag.txt",
	}, "", "")

	if len(vs) != 1 || vs[0].rule != "forbidden-tag" {
		t.Fatalf("got %v, want exactly one forbidden-tag violation", vs)
	}
}

func TestScanForbiddenTomlTag(t *testing.T) {
	t.Parallel()

	vs := scanFixture(t, map[string]string{
		"pkg/wire.go": "toml_tag.txt",
	}, "", "")

	if len(vs) != 1 || vs[0].rule != "forbidden-tag" {
		t.Fatalf("got %v, want exactly one forbidden-tag violation", vs)
	}
}

func TestScanYamlTagAllowlisted(t *testing.T) {
	t.Parallel()

	vs := scanFixture(t, map[string]string{
		"pkg/wire.go": "yaml_tag.txt",
	}, "", "pkg/wire.go\n")

	if len(vs) != 0 {
		t.Fatalf("allowlisted yaml tag produced violations: %v", vs)
	}
}

func TestScanGoldenJSONTagMissing(t *testing.T) {
	t.Parallel()

	vs := scanFixture(t, map[string]string{
		"pkg/wire.go": "golden_missing.txt",
	}, "pkg\twireStruct\tpkg/wire.go\n", "")

	if len(vs) != 1 || vs[0].rule != "golden-json" {
		t.Fatalf("got %v, want exactly one golden-json violation", vs)
	}

	if !strings.Contains(vs[0].msg, "wireStruct") || !strings.Contains(vs[0].msg, "Secret") {
		t.Errorf("message %q lacks struct or field name", vs[0].msg)
	}
}

func TestScanGoldenStructPresentWithTags(t *testing.T) {
	t.Parallel()

	vs := scanFixture(t, map[string]string{
		"pkg/wire.go": "golden_present.txt",
	}, "pkg\twireStruct\tpkg/wire.go\n", "")

	if len(vs) != 0 {
		t.Fatalf("tagged wire struct produced violations: %v", vs)
	}
}

func TestScanGoldenStructVanished(t *testing.T) {
	t.Parallel()

	vs := scanFixture(t, map[string]string{
		"pkg/other.go": "clean.txt",
	}, "pkg\twireStruct\tpkg/wire.go\n", "")

	if len(vs) != 1 || vs[0].rule != "golden-removed" {
		t.Fatalf("got %v, want exactly one golden-removed violation", vs)
	}

	if !strings.Contains(vs[0].msg, "wireStruct") {
		t.Errorf("message %q lacks the vanished struct name", vs[0].msg)
	}
}

func TestScanSkipsTestAndGeneratedFiles(t *testing.T) {
	t.Parallel()

	vs := scanFixture(t, map[string]string{
		"pkg/wire_test.go": "empty_tag.txt",
		"pkg/generated.go": "generated.txt",
	}, "", "")

	if len(vs) != 0 {
		t.Fatalf("test/generated files produced violations: %v", vs)
	}
}

func TestLoadGoldenMalformedLine(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "golden_wire_structs.txt"), "# header\nonly-one-column\n")

	if _, err := loadGolden(filepath.Join(root, "golden_wire_structs.txt")); err == nil {
		t.Fatal("malformed golden line accepted")
	}
}

func TestEachDataLineCommentsAndBlanks(t *testing.T) {
	t.Parallel()

	var lines []string

	root := t.TempDir()
	path := filepath.Join(root, "data.txt")
	writeFile(t, path, "# comment\n\npkg\tName\tpkg/x.go\n   \n")

	if err := eachDataLine(path, func(line string) error {
		lines = append(lines, line)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if len(lines) != 1 || lines[0] != "pkg\tName\tpkg/x.go" {
		t.Fatalf("eachDataLine yielded %v, want the single data line", lines)
	}
}
