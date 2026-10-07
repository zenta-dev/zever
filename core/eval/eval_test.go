package eval

import (
	"context"
	"errors"
	"testing"
)

func TestExactScorer(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		output   string
		expected string
		want     float64
	}{
		{"equal", "hi", "hi", 1},
		{"different", "hi", "bye", 0},
		{"empty", "", "", 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := ExactScorer(tc.output, tc.expected); got != tc.want {
				t.Fatalf("ExactScorer = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestContainsScorer(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		output   string
		expected string
		want     float64
	}{
		{"contains", "the answer is 42", "42", 1},
		{"missing", "the answer", "42", 0},
		{"empty expected", "anything", "", 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := ContainsScorer(tc.output, tc.expected); got != tc.want {
				t.Fatalf("ContainsScorer = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRunSuite(t *testing.T) {
	run := func(context.Context, string) (string, error) { return "ok", nil }

	report := RunSuite(t.Context(), run, []Case{
		{Name: "a", Input: "x", Expected: "ok"},
		{Name: "b", Input: "y", Expected: "nope"},
	}, ExactScorer)

	if len(report.Results) != 2 {
		t.Fatalf("results = %d, want 2", len(report.Results))
	}

	if report.Mean != 0.5 {
		t.Fatalf("mean = %v, want 0.5", report.Mean)
	}

	if report.Failed != 0 {
		t.Fatalf("failed = %d, want 0", report.Failed)
	}
}

func TestRunSuite_failureContinues(t *testing.T) {
	want := errors.New("boom")
	run := func(context.Context, string) (string, error) { return "", want }

	report := RunSuite(t.Context(), run, []Case{
		{Name: "a", Input: "x", Expected: "x"},
		{Name: "b", Input: "y", Expected: "y"},
	}, ExactScorer)

	if report.Failed != 2 {
		t.Fatalf("failed = %d, want 2", report.Failed)
	}

	for _, r := range report.Results {
		if r.Score != -1 || !errors.Is(r.Err, want) {
			t.Fatalf("result = %+v, want score -1 with boom", r)
		}
	}
}
