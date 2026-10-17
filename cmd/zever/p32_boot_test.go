package main

import (
	"strings"
	"testing"
)

// TestP32ScaffoldBoots proves the minimal scaffold wires exactly the stack
// the generated server boots: db, log and router as typed Providers in
// internal/app/wiring.go, a serve.Main entrypoint, the hello sample module,
// and env files. Anything heavier is an opt-in `zever add` recipe.
func TestP32ScaffoldBoots(t *testing.T) {
	repoRoot := repoRootAbs(t)
	workDir := t.TempDir()
	withWorkingDir(t, workDir)
	t.Setenv(zeverFrameworkPathEnv, repoRoot)

	prevTidy := runGoModTidyFunc
	runGoModTidyFunc = func(string, string) error { return nil }
	t.Cleanup(func() { runGoModTidyFunc = prevTidy })

	if err := runNew([]string{"bootapp", "--no-tidy"}); err != nil {
		t.Fatalf("runNew: %v", err)
	}

	wiring := readFile(t, "bootapp/internal/app/wiring.go")
	for _, b := range []string{"db", "log", "observability", "ratelimit", "router"} {
		if !strings.Contains(wiring, "/"+b+"/") {
			t.Errorf("wiring.go missing minimal service %q", b)
		}
	}

	for _, frag := range []string{
		"container.Provide(DB, dbsqlite.New)",
		"container.WrapNoErr(logslog.New)",
		"container.Provide(Router, routerstdhttp.New)",
		"var DB = db.DefaultRef()",
		"hello.New()",
	} {
		if !strings.Contains(wiring, frag) {
			t.Errorf("wiring.go missing %q", frag)
		}
	}

	mainSrc := readFile(t, "bootapp/cmd/server/main.go")
	if !strings.Contains(mainSrc, "serve.Main(app.Providers(), app.Modules())") {
		t.Errorf("server main.go does not boot via serve.Main:\n%s", mainSrc)
	}
}
