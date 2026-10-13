package main

import (
	"strings"
	"testing"
)

// TestP32ScaffoldBoots is the RED test for P3-2: default scaffold must
// include every battery the generated server unconditionally resolves.
func TestP32ScaffoldBoots(t *testing.T) {
	repoRoot := repoRootAbs(t)
	workDir := t.TempDir()
	withWorkingDir(t, workDir)
	t.Setenv(zeverFrameworkPathEnv, repoRoot)

	if err := runNew([]string{"bootapp", "--no-tidy"}); err != nil {
		t.Fatalf("runNew: %v", err)
	}

	yamlContent := readFile(t, "bootapp/zever.yaml")
	for _, b := range []string{"log", "router", "auth", "permission", "observability", "db", "ratelimit"} {
		if !strings.Contains(yamlContent, b+":") {
			t.Errorf("zever.yaml missing boot-required battery %q:\n%s", b, yamlContent)
		}
	}

	appGo := readFile(t, "bootapp/internal/app/app.go")
	for _, frag := range []string{"authjwt.Register()", "permissionnoop.Register()", "observabilitystdout.Register()", "dbsqlite.Register()", "ratelimitmemory.Register()"} {
		if !strings.Contains(appGo, frag) {
			t.Errorf("app.go missing %q", frag)
		}
	}
}
