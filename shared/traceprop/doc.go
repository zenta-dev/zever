// Package traceprop propagates W3C trace context and baggage across async
// boundaries.
//
// It owns Inject (context into headers) and Extract (headers into context)
// plus StartConsumeSpan (extract, then start a consumer child span), all over
// the OTel W3C traceparent/tracestate/baggage composite propagator. It
// performs no IO and manages no connections.
//
// Type safety: plain map[string]string headers keep both core/queue.Headers
// and core/eventbus.Headers assignable without conversions or core imports.
// Unsupported features fail closed: invalid or absent headers leave the
// context untouched, and a context without a valid span injects nothing.
//
// DX: call Inject on every produce path (Push, Publish) and Extract or
// StartConsumeSpan on every consume path (Pop consumers, Subscribe handlers).
// Headers are never mutated in place; Inject returns a copy.
//
// Container: no container accessor exists; adapters call these helpers
// directly. See container/README.md.
//
// Lifecycle: no IO and no ctx storage; spans returned by StartConsumeSpan
// must be ended by the caller (defer span.End()).
//
// Errors: no errors are reported; invalid input is a no-op by design.
//
// Security: headers may carry trace IDs only, never secrets; never log them.
//
// Performance: one small map copy per inject; extract allocates nothing when
// headers are empty.
//
// Concurrency: safe for concurrent use. No globals, no init wiring.
package traceprop
