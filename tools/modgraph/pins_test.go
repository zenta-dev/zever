package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const pinsGoMod = `module github.com/zenta-dev/zever/libs/app

go 1.27.0

require (
	github.com/zenta-dev/zever/libs/leaf v0.6.1
	github.com/zenta-dev/zever/libs/other v0.6.1 // indirect
	github.com/stretchr/testify v1.9.0
)

require github.com/zenta-dev/zever/libs/solo v0.6.1

replace (
	github.com/zenta-dev/zever/libs/leaf => ../leaf
	github.com/zenta-dev/zever/libs/other v0.6.1 => ../other
)

replace github.com/zenta-dev/zever/libs/solo => ../solo
`

func TestRewritePins(t *testing.T) {
	got, n := rewritePins(pinsGoMod, "v0.7.1")
	if n != 3 {
		t.Fatalf("rewritten = %d, want 3", n)
	}
	for _, want := range []string{
		"\tgithub.com/zenta-dev/zever/libs/leaf v0.7.1\n",
		"\tgithub.com/zenta-dev/zever/libs/other v0.7.1 // indirect\n",
		"require github.com/zenta-dev/zever/libs/solo v0.7.1\n",
		"\tgithub.com/stretchr/testify v1.9.0\n",
		"github.com/zenta-dev/zever/libs/other v0.6.1 => ../other",
		"github.com/zenta-dev/zever/libs/leaf => ../leaf",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rewritten go.mod lacks %q:\n%s", want, got)
		}
	}
	again, n2 := rewritePins(got, "v0.7.1")
	if n2 != 0 || again != got {
		t.Errorf("rewrite not idempotent: n=%d", n2)
	}
}

func TestStalePins(t *testing.T) {
	stale := stalePins(pinsGoMod, "v0.7.1")
	if len(stale) != 3 {
		t.Fatalf("stale = %v, want 3 entries", stale)
	}
	if s := stalePins(pinsGoMod, "v0.6.1"); len(s) != 0 {
		t.Errorf("stale at matching version = %v, want none", s)
	}
}

func pinsRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "libs", "leaf", "go.mod"),
		"module github.com/zenta-dev/zever/libs/leaf\n\ngo 1.27.0\n")
	mustWriteFile(t, filepath.Join(root, "libs", "app", "go.mod"),
		"module github.com/zenta-dev/zever/libs/app\n\ngo 1.27.0\n\nrequire github.com/zenta-dev/zever/libs/leaf v0.6.1\n\nreplace github.com/zenta-dev/zever/libs/leaf => ../leaf\n")
	mustWriteFile(t, filepath.Join(root, "cmd", "zever", "VERSION"), "0.7.1\n")
	return root
}

func TestRunCheckPinsStaleAndSet(t *testing.T) {
	root := pinsRepo(t)

	code, stdout, _ := runModgraph(t, []string{"-check-pins", "-root", root})
	if code != 1 || !strings.Contains(stdout, "libs/app") || !strings.Contains(stdout, "want v0.7.1") {
		t.Fatalf("check-pins = %d, stdout %q; want 1 naming libs/app and v0.7.1", code, stdout)
	}

	code, stdout, stderr := runModgraph(t, []string{"-set-pins", "-root", root})
	if code != 0 {
		t.Fatalf("set-pins = %d (stdout %q, stderr %q)", code, stdout, stderr)
	}
	data, err := os.ReadFile(filepath.Join(root, "libs", "app", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "libs/leaf v0.7.1") || !strings.Contains(string(data), "libs/leaf => ../leaf") {
		t.Errorf("go.mod after set-pins:\n%s", data)
	}

	if code, stdout, _ := runModgraph(t, []string{"-check-pins", "-root", root}); code != 0 {
		t.Fatalf("check-pins after set = %d, stdout %q", code, stdout)
	}
}

func TestRunCheckPinsExplicitVersion(t *testing.T) {
	root := pinsRepo(t)
	if code, _, _ := runModgraph(t, []string{"-check-pins", "-version", "v0.6.1", "-root", root}); code != 0 {
		t.Fatalf("check-pins -version v0.6.1 = %d, want 0", code)
	}
}

func TestRunCheckPinsMissingVersionFile(t *testing.T) {
	root := pinsRepo(t)
	if err := os.Remove(filepath.Join(root, "cmd", "zever", "VERSION")); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runModgraph(t, []string{"-check-pins", "-root", root}); code != 1 || !strings.Contains(stderr, "VERSION") {
		t.Fatalf("check-pins without VERSION = %d, stderr %q; want 1 mentioning VERSION", code, stderr)
	}
}
