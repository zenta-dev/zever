package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderSkill(t *testing.T) {
	t.Parallel()

	out := renderSkill(newRootCmd())

	for _, want := range []string{
		"---\nname: zever\n",
		"description: ",
		"# zever skill",
		"## Machine interfaces",
		"## Workflows",
		"## Safety",
		"## Commands",
		"zever --json",
		"zever --help --agent",
		"--dry-run",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("skill missing %q", want)
		}
	}
}

func TestRenderSkillMatchesCommittedSnapshot(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "skills", "zever", "SKILL.md"))
	if err != nil {
		t.Fatalf("read committed skill: %v", err)
	}

	if got := renderSkill(newRootCmd()); got != string(raw) {
		t.Fatal("renderSkill drifted from skills/zever/SKILL.md: regenerate with `zever docs --skill --dir skills/zever`")
	}
}

func TestDocsSkillWritesFile(t *testing.T) {
	dir := t.TempDir()

	if err := writeSkillFile(dir, newRootCmd()); err != nil {
		t.Fatalf("writeSkillFile: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		t.Fatalf("read written skill: %v", err)
	}

	if !strings.Contains(string(raw), "# zever skill") {
		t.Fatalf("written skill missing header:\n%s", raw)
	}
}
