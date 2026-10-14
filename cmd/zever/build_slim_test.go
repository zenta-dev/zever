package main

import (
	"bytes"
	"context"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// stubAdaptersSource is a miniature adapters.go exercising both
// registration shapes: explicit core.Register and no-arg Register().
const stubAdaptersSource = `package main

import (
	cachememory "github.com/zenta-dev/zever/adapters/cache/memory"
	queuememory "github.com/zenta-dev/zever/adapters/queue/memory"
	"github.com/zenta-dev/zever/core/cache"
	"github.com/zenta-dev/zever/core/queue"
	queredis "github.com/zenta-dev/zever/adapters/queue/redis"
)

func init() {
	_ = cache.Register(cache.Memory, cachememory.New)
	_ = queue.Register(queue.Memory, queuememory.New)
	queredis.Register()
}
`

// TestSlimAdaptersSourceKeepsSelected pins that filtering keeps the selected
// battery's imports and Register calls and drops the rest, while core
// imports stay.
func TestSlimAdaptersSourceKeepsSelected(t *testing.T) {
	t.Parallel()

	sel := []batterySelection{{Battery: "cache", Adapter: "memory"}}

	out, err := slimAdaptersSource(stubAdaptersSource, sel)
	if err != nil {
		t.Fatalf("slimAdaptersSource error = %v", err)
	}

	if !strings.Contains(out, "adapters/cache/memory") {
		t.Errorf("slim source missing kept import:\n%s", out)
	}

	if strings.Contains(out, "adapters/queue/memory") || strings.Contains(out, "adapters/queue/redis") {
		t.Errorf("slim source keeps dropped imports:\n%s", out)
	}

	if !strings.Contains(out, "core/cache") {
		t.Errorf("slim source drops kept core import:\n%s", out)
	}

	if strings.Contains(out, "core/queue") {
		t.Errorf("slim source keeps unreferenced core import:\n%s", out)
	}

	if strings.Contains(out, "queuememory") || strings.Contains(out, "queredis") {
		t.Errorf("slim source keeps dropped Register calls:\n%s", out)
	}

	if _, err := parser.ParseFile(token.NewFileSet(), "adapters.go", out, 0); err != nil {
		t.Errorf("slim source does not parse: %v\n%s", err, out)
	}
}

// TestSlimAdaptersSourceNilKeepsAll pins that a nil selection (full set)
// keeps every adapter import.
func TestSlimAdaptersSourceNilKeepsAll(t *testing.T) {
	t.Parallel()

	out, err := slimAdaptersSource(stubAdaptersSource, nil)
	if err != nil {
		t.Fatalf("slimAdaptersSource error = %v", err)
	}

	for _, want := range []string{"adapters/cache/memory", "adapters/queue/memory", "adapters/queue/redis"} {
		if !strings.Contains(out, want) {
			t.Errorf("full-set slim source missing %q:\n%s", want, out)
		}
	}
}

// TestAdapterNameForDirExceptions pins the consolidated-dir reverse mapping.
func TestAdapterNameForDirExceptions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		battery string
		dir     string
		want    string
	}{
		{"password", "argon2", "argon2id"},
		{"resilience", "inproc", "memory"},
		{"search", "db", "db"},
		{"cache", "memory", "memory"},
	}

	for _, c := range cases {
		if got := adapterNameForDir(c.battery, c.dir); got != c.want {
			t.Errorf("adapterNameForDir(%q, %q) = %q, want %q", c.battery, c.dir, got, c.want)
		}
	}
}

// TestRunBuildDryRunBuildsNothing pins that --dry-run resolves and reports
// without invoking the compiler.
func TestRunBuildDryRunBuildsNothing(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer

	called := false

	err := runBuildWith(t.Context(), buildOptions{
		adapters: "log,router",
		out:      "zever-slim",
		dryRun:   true,
		stdout:   &out,
		build: func(_ context.Context, _ string, _ []string) ([]byte, error) {
			called = true

			return nil, nil
		},
	})
	if err != nil {
		t.Fatalf("runBuildWith dry-run error = %v", err)
	}

	if called {
		t.Fatal("dry-run invoked the compiler")
	}

	if !strings.Contains(out.String(), "zever-slim") {
		t.Errorf("dry-run output missing out path:\n%s", out.String())
	}
}

// TestRunBuildUnknownAdapterFailsClosed pins the unknown error path.
func TestRunBuildUnknownAdapterFailsClosed(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer

	err := runBuildWith(t.Context(), buildOptions{
		adapters: "nope",
		out:      "zever-slim",
		stdout:   &out,
	})
	if err == nil {
		t.Fatal("runBuildWith nope = nil error, want unknown adapter error")
	}
}
