// Package prompt builds the text prompts agent loops and RAG pipelines send
// to language models.
//
// It owns system prompts, grounding templates, JSON-repair prompts,
// tool-error feedback, schema design, migration review, citation checks,
// tool planning, doctor triage, diagnostic repair, outbox triage, and
// extraction splits. Builders are pure string functions with no I/O and
// no dependencies.
package prompt
