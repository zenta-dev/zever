package parser

import (
	"testing"

	"github.com/zenta-dev/zever/dsl/ast"
)

// TestSyncTopLevelTracksBrackets verifies R27: syncTopLevel must track
// () and [] depth (not only {}) and must not mis-sync past a valid
// declaration after a bad one, even when the bad decl contains brackets
// or strings/comments that mention top-level keywords.
func TestSyncTopLevelTracksBrackets(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{
			name: "brackets in bad entity do not swallow next Good",
			src:  "entity Bad { [ ( ] ) !!! } entity Good { id: uuid }",
		},
		{
			name: "paren depth in bad decl",
			src:  "entity Bad { ( } entity Good { id: uuid }",
		},
		{
			name: "bracket depth in bad decl",
			src:  "entity Bad { [ } entity Good { id: uuid }",
		},
		{
			name: "string containing entity does not mis-sync",
			src:  "entity Bad { !!! \"entity Good { id: uuid }\" } entity Good { id: uuid }",
		},
		{
			name: "comment containing entity does not mis-sync",
			src:  "entity Bad { !!! } // entity Fake { id: uuid }\nentity Good { id: uuid }",
		},
		{
			name: "unterminated brackets then Good",
			src:  "entity Bad { [ [ [ } entity Good { id: uuid }",
		},
		{
			name: "mixed delimiters balanced",
			src:  "entity Bad { [ ( { } ) ] !!! } entity Good { id: uuid name: string }",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("ParseFile panicked on %q: %v", tc.name, r)
				}
			}()
			p := New("test.zen", []byte(tc.src))
			file, diags := p.ParseFile()
			if file == nil {
				t.Fatalf("ParseFile returned nil for %q", tc.name)
			}
			if len(diags) == 0 {
				t.Fatalf("expected at least one diagnostic for bad Bad decl in %q", tc.name)
			}
			foundGood := false
			for _, d := range file.Decls {
				if e, ok := d.(*ast.EntityDecl); ok && e.Name == "Good" {
					foundGood = true
					if len(e.Fields) == 0 {
						t.Fatalf("Good entity has 0 fields, expected recovery to preserve its body for %q", tc.name)
					}
				}
			}
			if !foundGood {
				t.Fatalf("expected to find Good entity after Bad in %q, got %d decls", tc.name, len(file.Decls))
			}
		})
	}
}

func TestSyncTopLevelNeverPanicsOrHangsWithBrackets(t *testing.T) {
	inputs := []string{
		"entity Bad { [ }",
		"entity Bad { ( }",
		"entity Bad { [ ( ) ] }",
		"[[[[",
		"]]]]",
		"([)]",
		"\"[ entity Good\"",
		"entity Bad { \"unterminated string } entity Good { id: uuid }",
	}

	for _, src := range inputs {
		t.Run(src, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panicked on %q: %v", src, r)
				}
			}()
			p := New("test.zen", []byte(src))
			f, _ := p.ParseFile()
			if f == nil {
				t.Fatalf("nil file for %q", src)
			}
		})
	}
}
