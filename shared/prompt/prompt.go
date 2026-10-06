package prompt

import (
	"fmt"
	"strings"
)

// SystemPrompt builds an agent identity prompt from a role and instructions.
// An empty role selects a generic helpful-assistant identity.
func SystemPrompt(role, instructions string) string {
	role = strings.TrimSpace(role)
	if role == "" {
		role = "a helpful assistant"
	}

	var b strings.Builder

	fmt.Fprintf(&b, "You are %s.\n", strings.TrimRight(role, "."))

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

// SchemaDesign builds a prompt asking for a .zen schema for a domain
// description, with entity/service naming guidance.
func SchemaDesign(domain string) string {
	return "Design a zever .zen schema for the following domain.\n" +
		"Use `entity Name { field: type }` blocks and `service Name` blocks with `rpc` operations. " +
		"Keep names singular PascalCase, mark primary keys, and declare relations on both sides.\n\nDomain:\n" +
		strings.TrimSpace(domain)
}

// MigrationReview builds a prompt asking for review of a schema migration
// plan against breaking-change rules.
func MigrationReview(plan string) string {
	return "Review this schema migration plan for breaking changes " +
		"(removed fields, renamed entities, altered types, dropped services).\n" +
		"Flag each break with its impact and suggest a compatible alternative.\n\nPlan:\n" +
		strings.TrimSpace(plan)
}

// CitationCheck builds a prompt asking whether every factual claim in an
// answer is supported by the cited context chunks.
func CitationCheck(question, answer string, chunks []string) string {
	var b strings.Builder

	b.WriteString("Check that every factual claim in the answer below is supported by the cited context. " +
		"Reply with SUPPORTED or list each unsupported claim.\n\nQuestion:\n")
	b.WriteString(strings.TrimSpace(question))
	b.WriteString("\n\nAnswer:\n")
	b.WriteString(strings.TrimSpace(answer))

	for i, chunk := range chunks {
		fmt.Fprintf(&b, "\n\n[%d] %s", i+1, strings.TrimSpace(chunk))
	}

	return b.String()
}

// ToolPlan builds a prompt asking for a step-by-step tool-use plan before
// executing, so the model commits to a strategy it can be held to. Blank
// tool entries are skipped.
func ToolPlan(task string, tools []string) string {
	var b strings.Builder

	b.WriteString("Plan how to accomplish the following task with the available tools. " +
		"List each step as `tool-name: purpose`. Do not execute anything yet.\n\nTask:\n")
	b.WriteString(strings.TrimSpace(task))
	b.WriteString("\n\nTools:\n")

	for _, tool := range tools {
		if trimmed := strings.TrimSpace(tool); trimmed != "" {
			b.WriteString("- " + trimmed + "\n")
		}
	}

	return b.String()
}

// DoctorTriage builds a prompt mapping `doctor --json` FAIL rows to
// config-file and environment fixes.
func DoctorTriage(doctorOutput string) string {
	return "The following `zever doctor --json` output lists batteries that " +
		"failed to resolve. For each FAIL row, name the `zever.yaml` key or " +
		"`ZEVER_*` environment variable that fixes it, or state when the " +
		"failure is expected (for example a battery needing a secret that " +
		"must be supplied, never invented).\n\nDoctor output:\n" +
		strings.TrimSpace(doctorOutput)
}

// DiagRepair builds a prompt turning `check`/`compile` diagnostics into a
// minimal `.zen` edit plan.
func DiagRepair(diags string) string {
	return "The following `zever check` diagnostics carry `file:line:col` " +
		"positions. For each diagnostic, propose the smallest `.zen` edit " +
		"that resolves it, quoting the exact lines to change.\n\nDiagnostics:\n" +
		strings.TrimSpace(diags)
}

// OutboxTriage builds a prompt interpreting `outbox status` plus `dlq list`
// output into a requeue-vs-purge decision. It reminds the model to mask
// DSNs before echoing them.
func OutboxTriage(statusOutput string) string {
	return "The following `outbox status` and `dlq list` output needs a " +
		"requeue-vs-purge decision per dead message. Recommend exactly one " +
		"action each, and mask DSNs (never echo secrets or connection " +
		"strings).\n\nOutbox output:\n" +
		strings.TrimSpace(statusOutput)
}

// ExtractSplit builds a prompt planning a single-module-per-file split when
// extraction is rejected for a multi-module file.
func ExtractSplit(extractErr string) string {
	return "Extraction failed because one file declares several modules. " +
		"Plan moving each module into its own file, preserving every " +
		"declaration verbatim (no AST reprint exists, so copies must be " +
		"byte-exact).\n\nExtraction error:\n" +
		strings.TrimSpace(extractErr)
}
