package container

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// compileFailTimeout bounds each fixture build. Fixture modules are tiny
// and fully cached; the timeout only guards against a hung toolchain.
const compileFailTimeout = 2 * time.Minute

// TestCompileFailFixtures asserts the misuse fixtures under
// testdata/compilefail do NOT compile. Each fixture is a separate tiny
// module (excluded from the workspace and ignored via testdata), built
// detached from the workspace (GOWORK=off) so the module graph resolves
// exactly as an out-of-tree consumer sees it. The proxy stays enabled so
// transitive public modules resolve like every other CI go command; the
// pinned go.sum files keep the build deterministic.
//
// The full CI job wiring these fixtures lands in stream G; this test is the
// local gate until then.
func TestCompileFailFixtures(t *testing.T) {
	tests := []struct {
		dir  string
		want string
	}{
		{dir: "wrongdep", want: "does not match inferred type"},
		{dir: "sealed", want: "does not implement container.Provider"},
	}

	for _, tt := range tests {
		t.Run(tt.dir, func(t *testing.T) {
			dir := filepath.Join("testdata", "compilefail", tt.dir)

			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
				t.Fatalf("fixture %s: %v", dir, err)
			}

			ctx, cancel := context.WithTimeout(t.Context(), compileFailTimeout)
			defer cancel()

			cmd := exec.CommandContext(ctx, "go", "build", "./...")
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "GOWORK=off")

			out, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("go build %s succeeded, want compile failure", dir)
			}

			if !strings.Contains(string(out), tt.want) {
				t.Fatalf("go build %s error = %q, want it to mention %q", dir, out, tt.want)
			}
		})
	}
}
