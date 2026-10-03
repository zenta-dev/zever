package orm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRawSQLOnlyViaEscapeHatch guards the raw-SQL boundary: a KindRaw/NRaw
// node or a render.RawExpr literal may only be constructed in unsafe.go (the
// UnsafeRaw/UnsafeIdent escape hatch) or render.go (the render layer that
// turns RawExpr into a node), never in any other production file. This keeps
// every raw-SQL site grep-able and routed through the audited escape hatch.
func TestRawSQLOnlyViaEscapeHatch(t *testing.T) {
	t.Parallel()

	for _, dir := range []string{".", "render", "migrate", "dialect"} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
				continue
			}
			name := e.Name()
			if name == "unsafe.go" || name == "render.go" || strings.HasSuffix(name, "_test.go") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"NRaw", "KindRaw", "RawExpr{"} {
				if strings.Contains(string(data), want) {
					t.Errorf("%s/%s constructs raw SQL directly (%q); route through orm.UnsafeRaw/UnsafeIdent", dir, name, want)
				}
			}
		}
	}
}
