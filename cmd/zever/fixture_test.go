package main

import (
	"os"
	"path/filepath"
	"testing"
)

// zeverUserSchema is the canonical valid schema fixture shared by db, dev,
// launch, migrate, serve, tinker, worker, and screen tests. It aliases the
// inspection suite's schema so there is exactly one copy of the text.
const zeverUserSchema = inspectUserSchema

// writeZeverFixture writes content to dir/name (creating parent dirs) and
// returns the full path. It mirrors writeInspectFixture for test files that
// predate the shared name.
func writeZeverFixture(t *testing.T, dir, name, content string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %q: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture %q: %v", path, err)
	}

	return path
}
