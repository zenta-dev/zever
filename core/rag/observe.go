package rag

// EventType identifies the kind of RAG event.
type EventType string

// RAG event types emitted during Engine operation.
const (
	// EventIngest fires when an ingest completes with the document count.
	EventIngest EventType = "ingest"
	// EventRetrieve fires when a retrieval completes with the source count.
	EventRetrieve EventType = "retrieve"
	// EventGenerate fires when grounded generation completes.
	EventGenerate EventType = "generate"
)

// Event is one observation from an Engine.
type Event struct {
	// Type identifies the event kind.
	Type EventType
	// Query is the retrieval query for retrieve events; empty otherwise.
	Query string
	// Count is the document or source count; zero otherwise.
	Count int
	// Err is the operation error; nil on success.
	Err error
}

// Observer receives engine events. A nil Observer disables observation.
// Observers must be goroutine-safe: methods on one Engine may run
// concurrently.
type Observer func(Event)
