package eval

import (
	"context"
	"math"
	"testing"
)

// BenchmarkExactScorer measures the byte-identical comparison hot path.
func BenchmarkExactScorer(b *testing.B) {
	output := "expected output string"
	expected := "expected output string"

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = ExactScorer(output, expected)
	}
}

// BenchmarkExactScorer_Mismatch measures the mismatch path.
func BenchmarkExactScorer_Mismatch(b *testing.B) {
	output := "actual output"
	expected := "expected output"

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = ExactScorer(output, expected)
	}
}

// BenchmarkContainsScorer measures the substring containment hot path.
func BenchmarkContainsScorer(b *testing.B) {
	output := "this is a longer output string containing the expected"
	expected := "expected"

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = ContainsScorer(output, expected)
	}
}

// BenchmarkContainsScorer_EmptyExpected measures the empty expected edge case.
func BenchmarkContainsScorer_EmptyExpected(b *testing.B) {
	output := "any output"
	expected := ""

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = ContainsScorer(output, expected)
	}
}

// BenchmarkClampScore measures the score bounding hot path.
func BenchmarkClampScore(b *testing.B) {
	scores := []float64{-1, -0.5, 0, 0.5, 1, 1.5, 2}
	nan := math.NaN()
	i := 0

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		s := scores[i%len(scores)]
		if i%8 == 0 {
			s = nan
		}
		_ = clampScore(s)
		i++
	}
}

// BenchmarkRunSuite_Success measures the full suite execution with
// all successful runs.
func BenchmarkRunSuite_Success(b *testing.B) {
	cases := make([]Case, 100)
	for i := range cases {
		cases[i] = Case{
			Name:     "case" + string(rune('0'+i%10)),
			Input:    "input",
			Expected: "expected",
		}
	}
	run := func(_ context.Context, _ string) (string, error) {
		return "expected", nil
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = RunSuite(b.Context(), run, cases, ExactScorer)
	}
}

// BenchmarkRunSuite_WithFailures measures suite execution with some
// failing runs.
func BenchmarkRunSuite_WithFailures(b *testing.B) {
	cases := make([]Case, 100)
	for i := range cases {
		cases[i] = Case{
			Name:     "case" + string(rune('0'+i%10)),
			Input:    "input",
			Expected: "expected",
		}
	}
	run := func(_ context.Context, input string) (string, error) {
		if input == "fail" {
			return "", context.Canceled
		}
		return "expected", nil
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = RunSuite(b.Context(), run, cases, ExactScorer)
	}
}

// BenchmarkRunSuite_Parallel measures concurrent suite execution.
func BenchmarkRunSuite_Parallel(b *testing.B) {
	cases := make([]Case, 10)
	for i := range cases {
		cases[i] = Case{
			Name:     "case" + string(rune('0'+i)),
			Input:    "input",
			Expected: "expected",
		}
	}
	run := func(_ context.Context, _ string) (string, error) {
		return "expected", nil
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = RunSuite(b.Context(), run, cases, ExactScorer)
		}
	})
}
