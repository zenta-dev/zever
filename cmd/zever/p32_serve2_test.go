package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestP32ServePreflightUnregisteredAdapter proves yaml battery without app.go Register fails fast.
func TestP32ServePreflightUnregisteredAdapter(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	if err := os.WriteFile("go.mod", []byte("module example.com/svc\n\ngo 1.27.2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("go.sum", []byte("dummy\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join("cmd", "server"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("cmd", "server", "main.go"), []byte("package main\nfunc main(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("zever.yaml", []byte("auth:\n  adapter: jwt\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join("internal", "app"), 0o750); err != nil {
		t.Fatal(err)
	}
	// app.go registers only log, not auth.
	if err := os.WriteFile(filepath.Join("internal", "app", "app.go"), []byte("package app\nfunc New(){ logslog.Register() }\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	calls := stubLaunch(t, nil)
	err := runServe(nil)
	if err == nil {
		t.Fatal("expected unregistered-adapter error")
	}
	if !strings.Contains(err.Error(), "zever add auth") {
		t.Fatalf("error %q lacks `zever add auth` fix", err)
	}
	if len(*calls) != 0 {
		t.Fatalf("launched despite pre-flight failure: %+v", *calls)
	}
}
