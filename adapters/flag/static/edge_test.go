package static

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zenta-dev/zever/core/flag"
)

// TestEdgeNewEmptyObjectJSON proves an empty JSON object yields an empty flag
// set (not an error) and every lookup falls back.
func TestEdgeNewEmptyObjectJSON(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "flags.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	f, err := New(flag.Options{Static: flag.StaticOptions{Path: path}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := t.Context()

	if got, err := f.Bool(ctx, "missing", true); err != nil || !got {
		t.Fatalf("Bool = %v, %v; want true, nil", got, err)
	}

	if got, err := f.String(ctx, "missing", "fb"); err != nil || got != "fb" {
		t.Fatalf("String = %q, %v; want %q, nil", got, err, "fb")
	}

	if got, err := f.Int(ctx, "missing", 9); err != nil || got != 9 {
		t.Fatalf("Int = %d, %v; want 9, nil", got, err)
	}
}
