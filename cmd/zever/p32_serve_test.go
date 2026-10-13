package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestP32ServePreflightMissingGoSum is RED for serve pre-flight.
func TestP32ServePreflightMissingGoSum(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	// Minimal project: go.mod without go.sum, entry exists.
	if err := os.WriteFile("go.mod", []byte("module example.com/svc\n\ngo 1.27.2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join("cmd", "server"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("cmd", "server", "main.go"), []byte("package main\nfunc main(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	calls := stubLaunch(t, nil)
	if err := runServe(nil); err == nil {
		t.Fatal("expected pre-flight error for missing go.sum")
	} else if !strings.Contains(err.Error(), "go mod tidy") {
		t.Fatalf("error %q lacks `go mod tidy` fix", err)
	}
	if len(*calls) != 0 {
		t.Fatalf("pre-flight failure launched child: %+v", *calls)
	}
}
