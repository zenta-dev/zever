// Package prompt builds the text prompts agent loops and RAG pipelines send
// to language models.
//
// It owns system prompts, grounding templates, JSON-repair prompts, and
// tool-error feedback. Builders are pure string functions with no I/O and
// no dependencies.
package prompt
