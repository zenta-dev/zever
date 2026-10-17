package main

import (
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestRenderNewMakefile(t *testing.T) {
	t.Parallel()

	mk := renderNewMakefile("gogen,openapi")

	for _, want := range []string{
		"BACKENDS ?= gogen,openapi",
		"\ngenerate:",
		"\n\tzever compile --backend=$(BACKENDS) --out ./generated $(SCHEMA)",
		"\ncheck-generated:",
		"\nbuild: generate",
		"\ntest: generate",
		"\nlint:",
		"\n\tgolangci-lint run ./...",
		"\nrun:",
		"\n\tgo run ./cmd/server",
		".PHONY:",
	} {
		if !strings.Contains(mk, want) {
			t.Errorf("Makefile lacks %q:\n%s", want, mk)
		}
	}

	// Make recipes must start with a tab, never spaces.
	for i, line := range strings.Split(mk, "\n") {
		if strings.HasPrefix(line, "  ") {
			t.Errorf("Makefile line %d is space-indented: %q", i+1, line)
		}
	}
}

func TestNewToolingConfigsParse(t *testing.T) {
	t.Parallel()

	for name, src := range map[string]string{
		".golangci.yml":  newGolangciYml,
		"dependabot.yml": newDependabotYml,
	} {
		var doc map[string]any
		if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
			t.Errorf("%s does not parse: %v", name, err)
		}
	}

	var dep struct {
		Version int `yaml:"version"`
		Updates []struct {
			Ecosystem string `yaml:"package-ecosystem"`
		} `yaml:"updates"`
	}

	if err := yaml.Unmarshal([]byte(newDependabotYml), &dep); err != nil {
		t.Fatalf("dependabot: %v", err)
	}

	got := map[string]bool{}
	for _, u := range dep.Updates {
		got[u.Ecosystem] = true
	}

	for _, want := range []string{"gomod", "github-actions", "docker"} {
		if !got[want] {
			t.Errorf("dependabot lacks ecosystem %q", want)
		}
	}

	if !strings.Contains(newGolangciYml, `version: "2"`) {
		t.Errorf(".golangci.yml must declare v2:\n%s", newGolangciYml)
	}

	if !strings.Contains(newEditorconfig, "root = true") || !strings.Contains(newEditorconfig, "[Makefile]") {
		t.Errorf(".editorconfig incomplete:\n%s", newEditorconfig)
	}
}
