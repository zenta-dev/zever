package eval

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/core/agent"
	"github.com/zenta-dev/zever/core/ai"
)

// JudgeScorer asks an ai.AI backend to rate an output against the expected
// answer on a 0..1 scale. It constrains the judge to a single number through
// agent.GenerateStructured and clamps the result. Judge failures return an
// error for the caller to record.
func JudgeScorer(client ai.AI, model string) func(ctx context.Context, output, expected string) (float64, error) {
	return func(ctx context.Context, output, expected string) (float64, error) {
		var out struct {
			Score float64 `json:"score"`
		}

		msgs := []ai.Message{{
			Role: ai.RoleUser,
			Content: "Rate the following answer against the reference on a 0 to 1 scale.\n\nReference:\n" +
				expected + "\n\nAnswer:\n" + output,
		}}

		schema := map[string]any{
			"type":       "object",
			"properties": map[string]any{"score": map[string]any{"type": "number"}},
			"required":   []string{"score"},
		}

		if _, err := agent.GenerateStructured(ctx, client, model, msgs, schema, &out); err != nil {
			return 0, fmt.Errorf("eval: judge failed: %w", err)
		}

		score := out.Score
		if score < 0 {
			score = 0
		}
		if score > 1 {
			score = 1
		}

		return score, nil
	}
}
