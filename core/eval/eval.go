package eval

import (
	"context"
	"math"
	"strings"
)

// Case is one evaluation case.
type Case struct {
	// Name identifies the case in reports.
	Name string
	// Input is fed to the run function.
	Input string
	// Expected is the reference answer scorers compare against.
	Expected string
}

// Scorer scores an output against the expected answer on a 0..1 scale.
type Scorer func(output, expected string) float64

// ExactScorer scores 1 for byte-identical output, 0 otherwise.
func ExactScorer(output, expected string) float64 {
	if output == expected {
		return 1
	}

	return 0
}

// ContainsScorer scores 1 when output contains expected, 0 otherwise. An
// empty expected always scores 0.
func ContainsScorer(output, expected string) float64 {
	if expected == "" {
		return 0
	}

	if strings.Contains(output, expected) {
		return 1
	}

	return 0
}

// RunFunc produces an output for an input. It mirrors the agent call shape
// without importing the agent package.
type RunFunc func(ctx context.Context, input string) (string, error)

// Result is one scored case outcome.
type Result struct {
	// CaseName identifies the case.
	CaseName string
	// Output is what the run function produced.
	Output string
	// Score is the scorer value, or -1 when the run failed.
	Score float64
	// Err is the run failure, if any.
	Err error
}

// Report aggregates a suite run.
type Report struct {
	// Results holds one outcome per case in dataset order.
	Results []Result
	// Mean is the mean score over successful runs; 0 with no successes.
	Mean float64
	// Failed counts run failures.
	Failed int
}

// clampScore bounds a scorer value to [0,1]; NaN scores 0 so one broken
// scorer cannot poison the mean.
func clampScore(score float64) float64 {
	if math.IsNaN(score) || score < 0 {
		return 0
	}

	if score > 1 {
		return 1
	}

	return score
}

// RunSuite executes every case through run, scores the outputs, and returns
// the report. A failing run records Score -1 and continues with the rest.
func RunSuite(ctx context.Context, run RunFunc, cases []Case, scorer Scorer) Report {
	report := Report{Results: make([]Result, 0, len(cases))}

	var (
		sum float64
		n   int
	)

	for _, c := range cases {
		if err := ctx.Err(); err != nil {
			report.Results = append(report.Results, Result{CaseName: c.Name, Score: -1, Err: err})
			report.Failed++

			continue
		}

		out, err := run(ctx, c.Input)
		if err != nil {
			report.Results = append(report.Results, Result{CaseName: c.Name, Score: -1, Err: err})
			report.Failed++

			continue
		}

		score := clampScore(scorer(out, c.Expected))
		report.Results = append(report.Results, Result{CaseName: c.Name, Output: out, Score: score})

		sum += score
		n++
	}

	if n > 0 {
		report.Mean = sum / float64(n)
	}

	return report
}
