package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// benchWorkflowPath locates .github/workflows/bench.yml from this source
// file, so the test works regardless of the test working directory.
func benchWorkflowPath(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller: no caller info")
	}

	path := filepath.Join(filepath.Dir(file), "..", "..", ".github", "workflows", "bench.yml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("bench workflow not found at %s: %v", path, err)
	}

	return path
}

// benchJob is one job entry in the bench workflow file.
type benchJob struct {
	Name          string `yaml:"name"`
	ContinueOnErr bool   `yaml:"continue-on-error"`
	RunsOn        string `yaml:"runs-on"`
}

// TestBenchWorkflowSyntax pins the P5-6 non-gating bench workflow: the YAML
// parses, the orm/container/benchstat jobs exist, and every job sets
// continue-on-error so benchmark noise can never gate a PR.
func TestBenchWorkflowSyntax(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(benchWorkflowPath(t))
	if err != nil {
		t.Fatalf("read bench workflow: %v", err)
	}

	var wf struct {
		On   any                 `yaml:"on"`
		Jobs map[string]benchJob `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		t.Fatalf("bench.yml is not valid YAML: %v", err)
	}

	for _, job := range []string{"bench-orm", "bench-container", "benchstat"} {
		j, ok := wf.Jobs[job]
		if !ok {
			t.Fatalf("bench.yml lacks job %q (jobs: %v)", job, keysOfJobs(wf.Jobs))
		}

		if !j.ContinueOnErr {
			t.Errorf("job %q lacks continue-on-error: true (benchmarks must not gate)", job)
		}

		if j.RunsOn == "" {
			t.Errorf("job %q has no runs-on", job)
		}
	}

	if on, ok := wf.On.(map[string]any); ok {
		if _, ok := on["pull_request"]; !ok {
			t.Errorf("bench.yml on: lacks pull_request (%v)", wf.On)
		}
	} else if s, ok := wf.On.(string); ok {
		if !strings.Contains(s, "pull_request") {
			t.Errorf("bench.yml on: = %q, want pull_request", s)
		}
	} else {
		t.Errorf("bench.yml on: = %T, want mapping with pull_request", wf.On)
	}

	text := string(raw)
	for _, want := range []string{"./orm/...", "./container/...", "benchstat"} {
		if !strings.Contains(text, want) {
			t.Errorf("bench.yml lacks %q", want)
		}
	}
}

func keysOfJobs(jobs map[string]benchJob) []string {
	out := make([]string, 0, len(jobs))
	for k := range jobs {
		out = append(out, k)
	}

	return out
}
