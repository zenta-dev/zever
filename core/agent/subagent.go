package agent

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/zenta-dev/zever/core/ai"
)

// AsTool wraps a sub-agent as a callable tool, enabling agent composition: a
// parent loop delegates a subtask by calling the tool, and the sub-agent runs
// it to completion. The handler marshals the call arguments to a JSON user
// message, runs sub, and returns its final content. Sub-agent failures
// propagate as errors, which the parent reports as a tool turn for
// self-correction.
func AsTool(name, description string, sub Agent, schema map[string]any) Tool {
	return Tool{
		Name:        name,
		Description: description,
		Parameters:  schema,
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			if sub == nil {
				return "", errors.New("agent: nil sub-agent")
			}

			raw, err := json.Marshal(args)
			if err != nil {
				return "", err
			}

			res, err := sub.Run(ctx, []ai.Message{{Role: ai.RoleUser, Content: string(raw)}})
			if err != nil {
				return "", err
			}

			return res.Content, nil
		},
	}
}
