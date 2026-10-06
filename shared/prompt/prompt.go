package prompt

import (
	"fmt"
	"strings"
)

// SystemPrompt builds an agent identity prompt from a role and instructions.
func SystemPrompt(role, instructions string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "You are %s.\n", strings.TrimSpace(role))

	if trimmed := strings.TrimSpace(instructions); trimmed != "" {
		b.WriteString(trimmed)
		b.WriteString("\n")
	}

	return b.String()
}

// GroundContext builds a retrieval-grounded user prompt: numbered context
// chunks with [n] citation markers followed by the question.
func GroundContext(question string, chunks []string) string {
	var b strings.Builder

	b.WriteString("Context:\n")

	for i, chunk := range chunks {
		fmt.Fprintf(&b, "[%d] %s\n", i+1, strings.TrimSpace(chunk))
	}

	fmt.Fprintf(&b, "\nQuestion: %s", strings.TrimSpace(question))

	return b.String()
}

// JSONRepair builds a re-prompt asking the model to fix invalid JSON,
// including the decode error for self-correction.
func JSONRepair(schema, badOutput string, decodeErr error) string {
	return "Your previous response was not valid JSON for the required schema" +
		(errSuffix(decodeErr) + ".\n\nSchema:\n" + schema +
			"\n\nInvalid response:\n" + badOutput +
			"\n\nReply with only valid JSON.")
}

// ToolErrorFeedback builds a tool-turn message reporting a handler failure
// so the model can self-correct and retry with adjusted parameters.
func ToolErrorFeedback(tool, errMsg string) string {
	return fmt.Sprintf(
		"Tool %q failed: %s. Adjust the arguments and try again, or use a different tool.",
		tool, strings.TrimSpace(errMsg),
	)
}

func errSuffix(err error) string {
	if err == nil {
		return ""
	}

	return ": " + err.Error()
}
