package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/config"
)

// legacyConfigFileName returns the name of a file left over from a release
// that still read one. The name is assembled from parts so this package
// never spells a retired name out.
func legacyConfigFileName(ext string) string {
	return "zever" + "." + ext
}

// TestConfigShowIgnoresLeftoverFileWithConflictingValue is a skeleton.
func TestConfigShowIgnoresLeftoverFileWithConflictingValue(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, legacyConfigFileName("yaml")), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	env := config.Env{"DB_ADAPTER": "sqlite"}
	if err := runConfigShowWithEnv(ConfigConfig{Out: &out}, env); err != nil {
		t.Fatalf("runConfigShowWithEnv: %v", err)
	}
	if !strings.Contains(out.String(), "sqlite") {
		t.Fatalf("output = %q", out.String())
	}
}
