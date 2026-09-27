package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

// TestSyntheticRepoLeafChange builds a temp git repo with module dirs and
// require edges, modifies a leaf, and asserts the exact affected set
// including transitive (direct and indirect) dependents.
func TestSyntheticRepoLeafChange(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()

	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
			"GIT_CONFIG_NOSYSTEM=1", "HOME="+root,
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	leafMod := "module github.com/zenta-dev/zever/libs/leaf\n\ngo 1.27.0\n"
	midMod := "module github.com/zenta-dev/zever/libs/mid\n\ngo 1.27.0\n\nrequire github.com/zenta-dev/zever/libs/leaf v0.0.0\n"
	topMod := "module github.com/zenta-dev/zever/libs/top\n\ngo 1.27.0\n\nrequire (\n\tgithub.com/zenta-dev/zever/libs/mid v0.0.0\n\tgithub.com/zenta-dev/zever/libs/leaf v0.0.0 // indirect\n)\n"
	otherMod := "module github.com/zenta-dev/zever/libs/other\n\ngo 1.27.0\n"

	git("init", "-b", "main", ".")
	write("libs/leaf/go.mod", leafMod)
	write("libs/leaf/leaf.go", "package leaf\n")
	write("libs/mid/go.mod", midMod)
	write("libs/mid/mid.go", "package mid\n")
	write("libs/top/go.mod", topMod)
	write("libs/top/top.go", "package top\n")
	write("libs/other/go.mod", otherMod)
	write("libs/other/other.go", "package other\n")
	git("add", "-A")
	git("commit", "-m", "base")

	// Leaf change on top of base.
	write("libs/leaf/leaf.go", "package leaf\n\n// changed\n")
	git("add", "-A")
	git("commit", "-m", "leaf change")

	modules, err := listModules(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"libs/leaf", "libs/mid", "libs/other", "libs/top"}; !reflect.DeepEqual(modules, want) {
		t.Fatalf("listModules = %#v, want %#v", modules, want)
	}
	requires, err := loadRequires(root, modules)
	if err != nil {
		t.Fatal(err)
	}

	mb, err := gitMergeBase(root, "HEAD~1")
	if err != nil {
		t.Fatal(err)
	}
	changed, err := gitChanged(root, mb)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"libs/leaf/leaf.go"}; !reflect.DeepEqual(changed, want) {
		t.Fatalf("changed = %#v, want %#v", changed, want)
	}

	groups, reason := classify(changed, modules, requires, 8)
	if reason != "affected" {
		t.Fatalf("reason = %q, want affected", reason)
	}
	var flat []string
	for _, g := range groups {
		flat = append(flat, g...)
	}
	want := []string{"libs/leaf", "libs/mid", "libs/top"}
	if !reflect.DeepEqual(flat, want) {
		t.Errorf("affected = %#v, want %#v (groups %#v)", flat, want, groups)
	}
}
