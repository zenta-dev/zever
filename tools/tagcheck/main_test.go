package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// goldenClean registers the wire struct of the clean fixture, and
// goldenCleanFields pins its wire keys.
const (
	goldenClean       = "pkg\twire\tpkg/clean.go\n"
	goldenCleanFields = "pkg\twire\tName\tname\npkg\twire\tKind\tkind\n"
)

// goldenStruct registers the wire struct of the wire fixtures, and
// goldenStructFields pins its wire key.
const (
	goldenStruct       = "pkg\twireStruct\tpkg/wire.go\n"
	goldenStructFields = "pkg\twireStruct\tName\tname\n"
)

// writeFile copies fixture content (testdata/*.txt) to path, failing the
// test on any error.
func writeFile(t *testing.T, path, content string) {
	t.Helper()

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}

	// Open the directory as a root so the file name can never escape it.
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()

	if err := root.WriteFile(filepath.Base(path), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// mustOpen opens path for reading, failing the test when it cannot.
func mustOpen(t *testing.T, path string) *os.File {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = f.Close() })

	return f
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
// golden is the wire-struct registry, fields the pinned wire keys, except
// the exception list, and allowlist the yaml:/toml: allowlist.
func scanFixture(t *testing.T, files map[string]string, golden, fields, except, allowlist string) []violation {
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

	toolDir := filepath.Join(root, "tools", "tagcheck")

	writeFile(t, filepath.Join(toolDir, "golden_wire_structs.txt"), golden)
	writeFile(t, filepath.Join(toolDir, "golden_wire_fields.txt"), fields)
	writeFile(t, filepath.Join(toolDir, "golden_wire_exceptions.txt"), except)
	writeFile(t, filepath.Join(toolDir, "tag_allowlist.txt"), allowlist)

	g, err := loadGolden(toolDir)
	if err != nil {
		t.Fatal(err)
	}

	f, err := loadGoldenFields(toolDir)
	if err != nil {
		t.Fatal(err)
	}

	e, err := loadExceptions(toolDir)
	if err != nil {
		t.Fatal(err)
	}

	a, err := loadAllowlist(toolDir)
	if err != nil {
		t.Fatal(err)
	}

	vs, err := scan(root, rules{structs: g, fields: f, except: e, allowed: a})
	if err != nil {
		t.Fatal(err)
	}

	return vs
}

func TestScanCleanFile(t *testing.T) {
	t.Parallel()

	vs := scanFixture(t, map[string]string{
		"pkg/clean.go": "clean.txt",
	}, goldenClean, goldenCleanFields, "", "")

	if len(vs) != 0 {
		t.Fatalf("clean file produced violations: %v", vs)
	}
}

func TestScanEmptyTag(t *testing.T) {
	t.Parallel()

	vs := scanFixture(t, map[string]string{
		"pkg/wire.go": "empty_tag.txt",
	}, "", "", "", "")

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
	}, goldenStruct, goldenStructFields, "", "")

	if len(vs) != 1 || vs[0].rule != "forbidden-tag" {
		t.Fatalf("got %v, want exactly one forbidden-tag violation", vs)
	}
}

func TestScanForbiddenTomlTag(t *testing.T) {
	t.Parallel()

	vs := scanFixture(t, map[string]string{
		"pkg/wire.go": "toml_tag.txt",
	}, goldenStruct, goldenStructFields, "", "")

	if len(vs) != 1 || vs[0].rule != "forbidden-tag" {
		t.Fatalf("got %v, want exactly one forbidden-tag violation", vs)
	}
}

func TestScanYamlTagAllowlisted(t *testing.T) {
	t.Parallel()

	vs := scanFixture(t, map[string]string{
		"pkg/wire.go": "yaml_tag.txt",
	}, goldenStruct, goldenStructFields, "", "pkg/wire.go\n")

	if len(vs) != 0 {
		t.Fatalf("allowlisted yaml tag produced violations: %v", vs)
	}
}

func TestScanGoldenJSONTagMissing(t *testing.T) {
	t.Parallel()

	vs := scanFixture(t, map[string]string{
		"pkg/wire.go": "golden_missing.txt",
	}, goldenStruct, "pkg\twireStruct\tName\tname\npkg\twireStruct\tSecret\tsecret\n", "", "")

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
	}, goldenStruct, "pkg\twireStruct\tName\tname\npkg\twireStruct\tSecret\tsecret\n", "", "")

	if len(vs) != 0 {
		t.Fatalf("tagged wire struct produced violations: %v", vs)
	}
}

func TestScanGoldenStructVanished(t *testing.T) {
	t.Parallel()

	vs := scanFixture(t, map[string]string{
		"pkg/other.go": "clean.txt",
	}, goldenClean+"pkg\twireStruct\tpkg/wire.go\n", goldenCleanFields, "", "")

	if len(vs) != 1 || vs[0].rule != "golden-removed" {
		t.Fatalf("got %v, want exactly one golden-removed violation", vs)
	}

	if !strings.Contains(vs[0].msg, "wireStruct") {
		t.Errorf("message %q lacks the vanished struct name", vs[0].msg)
	}
}

func TestScanGoldenKeyRenamed(t *testing.T) {
	t.Parallel()

	vs := scanFixture(t, map[string]string{
		"pkg/wire.go": "golden_renamed.txt",
	}, goldenStruct, "pkg\twireStruct\tName\tname\npkg\twireStruct\tError\terror\n", "", "")

	if len(vs) != 1 || vs[0].rule != "golden-key" {
		t.Fatalf("got %v, want exactly one golden-key violation", vs)
	}

	if !strings.Contains(vs[0].msg, "message") || !strings.Contains(vs[0].msg, "error") {
		t.Errorf("message %q does not name both the new and the golden key", vs[0].msg)
	}
}

func TestScanGoldenKeyEmpty(t *testing.T) {
	t.Parallel()

	vs := scanFixture(t, map[string]string{
		"pkg/wire.go": "golden_empty_key.txt",
	}, goldenStruct, "pkg\twireStruct\tName\tname\npkg\twireStruct\tError\terror\n", "", "")

	if len(vs) != 1 || vs[0].rule != "golden-key" {
		t.Fatalf("got %v, want exactly one golden-key violation", vs)
	}

	if !strings.Contains(vs[0].msg, "empty json key") {
		t.Errorf("message %q does not explain the empty json key", vs[0].msg)
	}
}

func TestScanGoldenKeyDropped(t *testing.T) {
	t.Parallel()

	vs := scanFixture(t, map[string]string{
		"pkg/wire.go": "golden_dropped.txt",
	}, goldenStruct, "pkg\twireStruct\tName\tname\npkg\twireStruct\tError\terror\n", "", "")

	if len(vs) != 1 || vs[0].rule != "golden-key" {
		t.Fatalf("got %v, want exactly one golden-key violation", vs)
	}

	if !strings.Contains(vs[0].msg, "dropped from the wire") {
		t.Errorf("message %q does not explain the dropped field", vs[0].msg)
	}
}

func TestScanGoldenKeyMissing(t *testing.T) {
	t.Parallel()

	vs := scanFixture(t, map[string]string{
		"pkg/wire.go": "golden_field_removed.txt",
	}, goldenStruct, "pkg\twireStruct\tName\tname\npkg\twireStruct\tError\terror\n", "", "")

	if len(vs) != 1 || vs[0].rule != "golden-missing" {
		t.Fatalf("got %v, want exactly one golden-missing violation", vs)
	}

	if !strings.Contains(vs[0].msg, "error") {
		t.Errorf("message %q does not name the lost wire key", vs[0].msg)
	}
}

func TestScanGoldenFieldRenamedKeepsWireKey(t *testing.T) {
	t.Parallel()

	vs := scanFixture(t, map[string]string{
		"pkg/wire.go": "golden_field_renamed.txt",
	}, goldenStruct, "pkg\twireStruct\tName\tname\npkg\twireStruct\tError\terror\n", "", "")

	if len(vs) != 0 {
		t.Fatalf("a renamed Go field that keeps its wire key produced violations: %v", vs)
	}
}

func TestScanGoldenStructWithoutPinnedKeys(t *testing.T) {
	t.Parallel()

	vs := scanFixture(t, map[string]string{
		"pkg/wire.go": "golden_present.txt",
	}, goldenStruct, "", "", "")

	if len(vs) != 1 || vs[0].rule != "golden-nofields" {
		t.Fatalf("got %v, want exactly one golden-nofields violation", vs)
	}
}

func TestScanUnregisteredWireStruct(t *testing.T) {
	t.Parallel()

	vs := scanFixture(t, map[string]string{
		"pkg/wire.go": "golden_present.txt",
	}, "", "", "", "")

	if len(vs) != 1 || vs[0].rule != "unregistered-wire" {
		t.Fatalf("got %v, want exactly one unregistered-wire violation", vs)
	}

	if !strings.Contains(vs[0].msg, "wireStruct") {
		t.Errorf("message %q does not name the unregistered struct", vs[0].msg)
	}
}

func TestScanUnregisteredWireStructExcepted(t *testing.T) {
	t.Parallel()

	vs := scanFixture(t, map[string]string{
		"pkg/wire.go": "hybrid.txt",
	}, "", "", "pkg\thybridStruct\tGo-API-only handle\n", "")

	if len(vs) != 0 {
		t.Fatalf("excepted wire struct produced violations: %v", vs)
	}
}

func TestScanExceptionForRegisteredStruct(t *testing.T) {
	t.Parallel()

	vs := scanFixture(t, map[string]string{
		"pkg/wire.go": "golden_present.txt",
	}, goldenStruct, goldenStructFields, "pkg\twireStruct\tstale reason\n", "")

	if len(vs) != 1 || vs[0].rule != "exception-registered" {
		t.Fatalf("got %v, want exactly one exception-registered violation", vs)
	}
}

func TestScanExceptionForFullyTaggedStruct(t *testing.T) {
	t.Parallel()

	vs := scanFixture(t, map[string]string{
		"pkg/wire.go": "golden_present.txt",
	}, "", "", "pkg\twireStruct\tGo-API-only handle\n", "")

	if len(vs) != 1 || vs[0].rule != "exception-unnecessary" {
		t.Fatalf("got %v, want exactly one exception-unnecessary violation", vs)
	}
}

func TestScanExceptionForVanishedStruct(t *testing.T) {
	t.Parallel()

	vs := scanFixture(t, map[string]string{
		"pkg/clean.go": "clean.txt",
	}, goldenClean, goldenCleanFields, "pkg\tgoneStruct\tstale reason\n", "")

	if len(vs) != 1 || vs[0].rule != "exception-stale" {
		t.Fatalf("got %v, want exactly one exception-stale violation", vs)
	}
}

func TestScanSkipsTestAndGeneratedFiles(t *testing.T) {
	t.Parallel()

	vs := scanFixture(t, map[string]string{
		"pkg/wire_test.go": "empty_tag.txt",
		"pkg/generated.go": "generated.txt",
	}, "", "", "", "")

	if len(vs) != 0 {
		t.Fatalf("test/generated files produced violations: %v", vs)
	}
}

func TestLoadGoldenMalformedLine(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "golden_wire_structs.txt"), "# header\nonly-one-column\n")

	if _, err := loadGolden(root); err == nil {
		t.Fatal("malformed golden line accepted")
	}
}

func TestLoadGoldenFieldsMalformedLine(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "golden_wire_fields.txt"), "# header\npkg\twireStruct\tName\n")

	if _, err := loadGoldenFields(root); err == nil {
		t.Fatal("malformed golden field line accepted")
	}
}

func TestEachDataLineCommentsAndBlanks(t *testing.T) {
	t.Parallel()

	var lines []string

	root := t.TempDir()
	path := filepath.Join(root, "data.txt")
	writeFile(t, path, "# comment\n\npkg\tName\tpkg/x.go\n   \n")

	if err := eachDataLine(mustOpen(t, path), path, func(line string) error {
		lines = append(lines, line)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if len(lines) != 1 || lines[0] != "pkg\tName\tpkg/x.go" {
		t.Fatalf("eachDataLine yielded %v, want the single data line", lines)
	}
}
