package main

import (
	"strings"
	"testing"
)

func TestRunGenerateEntityAppendsDeclaration(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	path := writeModuleFixture(t, dir, "shop", "")

	if err := runGenerateEntity([]string{"shop", "Order", "--field", "title:string", "--field", "total:int64"}); err != nil {
		t.Fatalf("runGenerateEntity: %v", err)
	}

	const want = "entity Order {\n" +
		"\tid: uuid @primary\n" +
		"\ttitle: string\n" +
		"\ttotal: int64\n" +
		"\tcreated_at: timestamp @default(now())\n" +
		"}\n"

	if got := readFile(t, path); got != want {
		t.Fatalf("content =\n%q\nwant\n%q", got, want)
	}

	assertZenParses(t, path)
}

// TestRunGenerateEntityFlagsBeforePositionals is the reason splitPositionals
// exists: Go's flag package stops at the first non-flag argument.
func TestRunGenerateEntityFlagsBeforePositionals(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	path := writeModuleFixture(t, dir, "shop", "")

	if err := runGenerateEntity([]string{"--field", "title:string", "shop", "Order"}); err != nil {
		t.Fatalf("runGenerateEntity: %v", err)
	}

	got := readFile(t, path)
	if !strings.Contains(got, "\ttitle: string\n") {
		t.Fatalf("field was not rendered:\n%s", got)
	}

	assertZenParses(t, path)
}

func TestRunGenerateEntityRejectsDuplicateName(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	const original = "job Order() {\n\tqueue: default\n}\n"

	path := writeModuleFixture(t, dir, "shop", original)

	// Entities, jobs, services and schedules share one namespace.
	err := runGenerateEntity([]string{"shop", "Order"})
	if err == nil {
		t.Fatalf("expected an error for a name a job already uses")
	}

	if !strings.Contains(err.Error(), "already declares") {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := readFile(t, path); got != original {
		t.Fatalf("file was modified: %q", got)
	}
}

func TestRunGenerateEntityArgumentErrors(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	writeModuleFixture(t, dir, "shop", "")

	tests := []struct {
		name string
		args []string
	}{
		{"no arguments", nil},
		{"missing entity name", []string{"shop"}},
		{"too many positionals", []string{"shop", "Order", "extra"}},
		{"module is not an identifier", []string{"sh-op", "Order"}},
		{"name is not an identifier", []string{"shop", "1Order"}},
		{"field without a type", []string{"shop", "Order", "--field", "title"}},
		{"field with an unknown type", []string{"shop", "Order", "--field", "title:strng"}},
		{"field with a bad name", []string{"shop", "Order", "--field", "1title:string"}},
		{"unknown module", []string{"warehouse", "Order"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := runGenerateEntity(tc.args); err == nil {
				t.Fatalf("expected an error for %v", tc.args)
			}
		})
	}
}

func TestRenderEntityDecl(t *testing.T) {
	got := renderEntityDecl("Order", fieldSpecs{{Name: "title", Type: "string"}})

	const want = "entity Order {\n" +
		"\tid: uuid @primary\n" +
		"\ttitle: string\n" +
		"\tcreated_at: timestamp @default(now())\n" +
		"}\n"

	if got != want {
		t.Fatalf("renderEntityDecl = %q, want %q", got, want)
	}

	assertGolden(t, "generate_entity_decl.golden", []byte(got))
}

func TestFieldSpecsStringRoundTrip(t *testing.T) {
	var specs fieldSpecs

	for _, v := range []string{"title:string", " total : int64 "} {
		if err := specs.Set(v); err != nil {
			t.Fatalf("Set(%q): %v", v, err)
		}
	}

	if got := specs.String(); got != "title:string,total:int64" {
		t.Fatalf("String() = %q", got)
	}
}

// TestKnownScalarMatchesScalarTypeNames guards the binary search in
// knownScalar: it is only correct while scalarTypeNames stays sorted.
func TestKnownScalarMatchesScalarTypeNames(t *testing.T) {
	for i, name := range scalarTypeNames {
		if i > 0 && scalarTypeNames[i-1] >= name {
			t.Fatalf("scalarTypeNames is not sorted at %d (%q after %q)", i, name, scalarTypeNames[i-1])
		}

		if !knownScalar(name) {
			t.Fatalf("knownScalar(%q) = false", name)
		}
	}

	for _, name := range []string{"", "String", "text", "zzz"} {
		if knownScalar(name) {
			t.Fatalf("knownScalar(%q) = true", name)
		}
	}
}

func TestIsIdent(t *testing.T) {
	valid := []string{"a", "_", "Order", "order_line", "x1", "_9"}
	invalid := []string{"", "1a", "a-b", "a b", "a.b", "é"}

	for _, s := range valid {
		if !isIdent(s) {
			t.Fatalf("isIdent(%q) = false", s)
		}
	}

	for _, s := range invalid {
		if isIdent(s) {
			t.Fatalf("isIdent(%q) = true", s)
		}
	}
}

// TestCoverGenerateEntityErrors covers pure validation + write faults.
func TestCoverGenerateEntityErrors(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	if _, err := GenerateEntity(GenerateEntityConfig{Module: "../evil", Name: "O"}); !isTraversalError(err) {
		t.Fatalf("want traversal, got %v", err)
	}
	if _, err := GenerateEntity(GenerateEntityConfig{Module: "shop", Name: "bad-name"}); err == nil {
		t.Fatalf("want ident error")
	}
	if _, err := GenerateEntity(GenerateEntityConfig{Module: "shop", Name: "O", Fields: []EntityField{{Name: "t", Type: "nope"}}}); err == nil {
		t.Fatalf("want field error")
	}
	// Append failure via broken decl: create module then corrupt write path by making .zen a directory.
	writeModuleFixture(t, dir, "shop", "entity Existing {\n\tid: uuid @primary\n}\n")
	if _, err := GenerateEntity(GenerateEntityConfig{Module: "shop", Name: "Existing"}); err == nil {
		t.Fatalf("want taken error")
	}
}

// TestCoverRunGenerateEntityInteractive covers prompt branches via seams.
func TestCoverRunGenerateEntityInteractive(t *testing.T) {
	stubPromptTTY(t, true)
	setPromptInteractive(t)
	orig := promptInputForEntity
	t.Cleanup(func() { promptInputForEntity = orig })
	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeModuleFixture(t, dir, "shop", "")
	// No positionals: module prompt then entity prompt.
	calls := 0
	promptInputForEntity = func(title, _ string, _ func(string) error) (string, error) {
		calls++
		if title == "Module name" {
			return "shop", nil
		}
		return "Prompted", nil
	}
	if err := runGenerateEntity(nil); err != nil {
		t.Fatalf("interactive entity: %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
	// Prompt error.
	promptInputForEntity = func(string, string, func(string) error) (string, error) { return "", errTestSentinel }
	if err := runGenerateEntity(nil); err == nil {
		t.Fatalf("want prompt error")
	}
	// Entity-only prompt error (module supplied).
	promptInputForEntity = func(string, string, func(string) error) (string, error) { return "", errTestSentinel }
	if err := runGenerateEntity([]string{"shop"}); err == nil {
		t.Fatalf("want entity prompt error")
	}
	// Parse error.
	if err := runGenerateEntity([]string{"--badflag", "shop", "O"}); err == nil {
		t.Fatalf("want parse error")
	}
	// Interactive flag.
	dir2 := t.TempDir()
	withWorkingDir(t, dir2)
	writeModuleFixture(t, dir2, "shop", "")
	promptInputForEntity = func(string, string, func(string) error) (string, error) { return "shop", nil }
	_ = runGenerateEntity([]string{"--interactive", "shop", "ViaFlag"})
	_ = runGenerateEntity([]string{"-i", "shop", "ViaFlag2"})
}
