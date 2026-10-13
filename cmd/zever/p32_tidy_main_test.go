package main

import (
	"os"
	"testing"
)

// TestMain stubs go mod tidy for unit tests so `zever new` stays fast and
// offline. Tests that verify tidy behavior stub runGoModTidyFunc themselves;
// the e2e boot test restores the real runner.
func TestMain(m *testing.M) {
	orig := runGoModTidyFunc
	runGoModTidyFunc = func(string, string) error { return nil }
	code := m.Run()
	runGoModTidyFunc = orig
	os.Exit(code)
}
