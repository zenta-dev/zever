// Package rag wires ai embeddings, a vectorstore and ai generation into a
// retrieval-augmented generation engine.
//
// It owns chunking, ingestion, retrieval and grounded generation. It does not
// own the LLM transport, the embedding provider, or the vector backend; callers
// pass a resolved ai.AI and vectorstore.VectorStore. Engines built by
// NewHybrid additionally index into a search backend for fused hybrid
// retrieval, and report operation events to Options.Observe.
//
// Type safety: Engine plus typed Options, Document, Source and Answer. Empty
// queries and malformed documents fail closed with sentinel errors.
//
// DX: build an Engine with New, ingest with Ingest, retrieve with Retrieve, or
// answer grounded in retrieved context with Answer. TopK, ChunkSize and
// ChunkOverlap resolve to package defaults when unset.
//
// Container: container.New(cfg) resolves ai and vectorstore; build an Engine
// over them with New. The engine holds no resources and has no Close.
//
// Lifecycle: ctx is first arg for IO, never stored. Methods are safe to call
// concurrently on one Engine.
//
// Errors: sentinel errors, errors.Is compatible, prefixed rag:. Errors name the
// field or count only, never document content.
//
// Security: retrieved content is untrusted; never log raw documents or option
// maps. Answer grounds generation in retrieved context.
//
// Performance: ingestion embeds chunks in one batch per Ingest call; retrieval
// embeds the query once. Bounded by TopK.
//
// Concurrency: safe for concurrent use unless noted. No globals, no init wiring.
//
// Example: see ExampleEngine in example_test.go.
package rag
