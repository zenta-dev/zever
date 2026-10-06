// Package agent runs bounded tool-calling LLM loops over an ai.AI backend.
//
// It owns the generate -> dispatch tool calls -> append tool results -> repeat
// loop, plus structured-output generation and an optional human-in-the-loop
// confirmation hook. It does not own the LLM transport or provider credentials;
// callers pass a resolved ai.AI.
//
// Type safety: Agent plus typed Options, Tool, ToolHandler and Result.
// Unknown tools fail closed with ErrUnknownTool; exceeding MaxSteps fails with
// ErrMaxSteps. Unsupported features fail closed.
//
// DX: build a Loop with New, or generate constrained JSON with
// GenerateStructured. Options carry the tool set and loop bound; MaxSteps <= 0
// selects DefaultMaxSteps. See config/README.md for AI service wiring.
//
// Container: container.New(cfg) resolves the ai backend; build an agent over it
// with New. The loop holds no resources and has no Close.
//
// Lifecycle: ctx is first arg for IO, never stored. Run is safe to call
// concurrently on one Loop.
//
// Errors: sentinel errors, errors.Is compatible, prefixed agent:. Errors name
// the tool or field only, never argument values.
//
// Security: never log secrets or raw option maps. Tool arguments and results
// are untrusted; destructive tools may require confirmation via Confirm before
// execution.
//
// Performance: MaxSteps bounds provider calls; conversation history is the only
// per-run allocation. Tool calls within a step run sequentially.
//
// Concurrency: safe for concurrent use unless noted. No globals, no init wiring.
//
// Example: see ExampleNew in example_test.go.
package agent
