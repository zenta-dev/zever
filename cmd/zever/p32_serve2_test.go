package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestP32ServePreflightIgnoresLegacyYaml proves a legacy zever.yaml battery
// without an app.go Register no longer gates serving: config is env-only,
// so the entrypoint launches.
func TestP32ServePreflightIgnoresLegacyYaml(t *testing.T) {
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
	if err := runServe(nil); err != nil {
		t.Fatalf("runServe: %v", err)
	}
	assertOneLaunch(t, calls, defaultServerEntry, nil)
}
