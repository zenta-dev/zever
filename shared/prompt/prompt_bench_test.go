package prompt

import (
	"testing"
)

// BenchmarkSystemPrompt measures the system prompt construction hot path.
func BenchmarkSystemPrompt(b *testing.B) {
	role := "a helpful assistant"
	instructions := "You must follow these rules:\n1. Be concise\n2. Be accurate\n3. Never hallucinate"

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = SystemPrompt(role, instructions)
	}
}

// BenchmarkSystemPrompt_EmptyRole measures the empty role fallback path.
func BenchmarkSystemPrompt_EmptyRole(b *testing.B) {
	role := ""
	instructions := "Some instructions"

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = SystemPrompt(role, instructions)
	}
}

// BenchmarkGroundContext measures the context grounding hot path with
// multiple chunks.
func BenchmarkGroundContext(b *testing.B) {
	question := "What is the capital of France?"
	chunks := []string{
		"Paris is the capital and most populous city of France.",
		"It is located on the Seine River in the north of the country.",
		"Paris has been a major center of finance, commerce, fashion, and culture.",
		"The city covers an area of 105.4 square kilometers.",
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = GroundContext(question, chunks)
	}
}

// BenchmarkGroundContext_EmptyChunks measures the edge case with no context.
func BenchmarkGroundContext_EmptyChunks(b *testing.B) {
	question := "What is the capital of France?"
	chunks := []string{}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = GroundContext(question, chunks)
	}
}

// BenchmarkJSONRepair measures the JSON repair prompt construction.
func BenchmarkJSONRepair(b *testing.B) {
	schema := `{"type":"object","properties":{"name":{"type":"string"}}}`
	badOutput := `{name: "missing quotes"}`
	decodeErr := &jsonSyntaxError{msg: "invalid character 'n' looking for beginning of value"}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = JSONRepair(schema, badOutput, decodeErr)
	}
}

// BenchmarkToolErrorFeedback measures the tool error feedback prompt.
func BenchmarkToolErrorFeedback(b *testing.B) {
	tool := "database_query"
	errMsg := "connection timeout after 30s"

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = ToolErrorFeedback(tool, errMsg)
	}
}

// BenchmarkSchemaDesign measures the schema design prompt construction.
func BenchmarkSchemaDesign(b *testing.B) {
	domain := "An e-commerce platform with users, products, orders, and payments."

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = SchemaDesign(domain)
	}
}

// BenchmarkMigrationReview measures the migration review prompt.
func BenchmarkMigrationReview(b *testing.B) {
	plan := "1. Rename entity User to Customer\n2. Add field email to Product\n3. Drop service Analytics"

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = MigrationReview(plan)
	}
}

// BenchmarkCitationCheck measures the citation check prompt with multiple
// context chunks.
func BenchmarkCitationCheck(b *testing.B) {
	question := "What is the population of Paris?"
	answer := "Paris has a population of 2.1 million people."
	chunks := []string{
		"Paris is the capital of France with a population of 2.1 million.",
		"The metropolitan area has over 12 million residents.",
		"Paris is known for its museums, fashion, and cuisine.",
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = CitationCheck(question, answer, chunks)
	}
}

// BenchmarkToolPlan measures the tool plan prompt with multiple tools.
func BenchmarkToolPlan(b *testing.B) {
	task := "Find the user's recent orders and send a summary email."
	tools := []string{"database_query", "email_send", "user_lookup", "order_lookup"}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = ToolPlan(task, tools)
	}
}

// BenchmarkDoctorTriage measures the doctor triage prompt.
func BenchmarkDoctorTriage(b *testing.B) {
	doctorOutput := `{"batteries":[{"name":"db","status":"FAIL","error":"no DSN"},{"name":"auth","status":"OK"}]}`

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = DoctorTriage(doctorOutput)
	}
}

// BenchmarkDiagRepair measures the diagnostic repair prompt.
func BenchmarkDiagRepair(b *testing.B) {
	diags := `schema/app.zen:10:5: undefined entity "User"\nschema/app.zen:15:3: field "email" missing type`

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = DiagRepair(diags)
	}
}

// BenchmarkOutboxTriage measures the outbox triage prompt.
func BenchmarkOutboxTriage(b *testing.B) {
	statusOutput := `{"status":"pending","dlq":[{"id":"msg-1","error":"timeout"}]}`

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = OutboxTriage(statusOutput)
	}
}

// BenchmarkExtractSplit measures the extract split prompt.
func BenchmarkExtractSplit(b *testing.B) {
	extractErr := "file declares multiple modules: module A, module B"

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = ExtractSplit(extractErr)
	}
}

// jsonSyntaxError is a minimal error type for benchmarking.
type jsonSyntaxError struct {
	msg string
}

func (e *jsonSyntaxError) Error() string { return e.msg }
