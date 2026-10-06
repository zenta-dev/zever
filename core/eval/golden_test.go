package eval

import (
	"context"
	"path/filepath"
	"testing"
)

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
		if c.Name == "" || c.Input == "" || c.Expected == "" {
			t.Fatalf("golden case missing name/input/expected: %+v", c)
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

	// The echo stub scores the honesty of the harness, not quality: every
	// result must carry the scorer's verdict for its own output.
	for i, r := range report.Results {
		want := ContainsScorer("echo", cases[i].Expected)
		if r.Score != want {
			t.Errorf("result %d score = %v, want %v", i, r.Score, want)
		}
	}
}

func TestLoadCases_missing(t *testing.T) {
	t.Parallel()

	if _, err := LoadCases(filepath.Join("testdata", "nope.json")); err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}
