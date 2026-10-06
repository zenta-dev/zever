package rag

import (
	"testing"
)

func TestObserve_ingestRetrieve(t *testing.T) {
	var events []Event
	obs := func(ev Event) { events = append(events, ev) }

	f := &fakeAI{}
	s := &memStore{}
	e := newTestEngine(t, f, s, Options{Observe: obs})

	if err := e.Ingest(t.Context(), []Document{{ID: "d", Content: "hello"}}); err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}

	if _, err := e.Retrieve(t.Context(), "hello", 3); err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}

	if events[0].Type != EventIngest || events[0].Count != 1 {
		t.Fatalf("first event = %+v, want ingest count 1", events[0])
	}

	if events[1].Type != EventRetrieve || events[1].Query != "hello" {
		t.Fatalf("second event = %+v, want retrieve hello", events[1])
	}
}
