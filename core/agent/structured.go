package agent

import (
	"context"
	"encoding/json"

	"github.com/zenta-dev/zever/core/ai"
)

// GenerateStructured asks client for a completion constrained to schema and
// decodes it into out. On a decode failure it re-prompts with the error, up to
// DefaultMaxRetries attempts, and returns ErrStructuredOutput when exhausted.
func GenerateStructured(
	ctx context.Context,
	client ai.AI,
	model string,
	messages []ai.Message,
	schema map[string]any,
	out any,
) (ai.Generation, error) {
	if client == nil {
		return ai.Generation{}, ErrNilClient
	}
	if out == nil {
		return ai.Generation{}, InvalidOptionsError{Reason: "out is required"}
	}

	msgs := make([]ai.Message, 0, len(messages)+2)
	msgs = append(msgs, messages...)

	var lastErr error

	for attempt := 0; attempt < DefaultMaxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return ai.Generation{}, err
		}

		gen, err := client.Generate(ctx, model, msgs, ai.GenerateOptions{
			ResponseFormat: &ai.ResponseFormat{Type: "json_schema", JSONSchema: schema},
		})
		if err != nil {
			return ai.Generation{}, err
		}

		decodeErr := json.Unmarshal([]byte(gen.Content), out)
		if decodeErr == nil {
			return gen, nil
		}
		lastErr = decodeErr

		msgs = append(msgs,
			ai.Message{Role: ai.RoleAssistant, Content: gen.Content},
			ai.Message{
				Role: ai.RoleUser,
				Content: "Your previous response was not valid JSON for the required " +
					"schema: " + lastErr.Error() + ". Reply with only valid JSON.",
			},
		)
	}

	return ai.Generation{}, StructuredOutputError{Cause: lastErr}
}
