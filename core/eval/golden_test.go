package eval

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// LoadCases reads a golden dataset of cases from path.
func LoadCases(path string) ([]Case, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cases []Case
	if err := json.Unmarshal(raw, &cases); err != nil {
		return nil, err
	}

	return cases, nil
}

// TestGolden runs the committed dataset through an echo run function scored
// by containment. It pins the dataset shape and the harness end to end;
// live-model evals run manually via JudgeScorer.
func TestGolden(t *testing.T) {
	t.Parallel()

	cases, err := LoadCases(filepath.Join("testdata", "cases.json"))
	if err != nil {
		t.Fatalf("LoadCases() error = %v", err)
	}

	if len(cases) == 0 {
		t.Fatal("no golden cases loaded")
	}

	for _, c := range cases {
		if c.Name == "" || c.Input == "" {
			t.Fatalf("golden case missing name/input: %+v", c)
		}
	}

	echo := func(context.Context, string) (string, error) { return "echo", nil }

	report := RunSuite(t.Context(), echo, cases, ContainsScorer)

	if report.Failed != 0 {
		t.Fatalf("failed = %d, want 0", report.Failed)
	}

	if len(report.Results) != len(cases) {
		t.Fatalf("results = %d, want %d", len(report.Results), len(cases))
	}
}

func TestLoadCases_missing(t *testing.T) {
	t.Parallel()

	if _, err := LoadCases(filepath.Join("testdata", "nope.json")); err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}
