package agent

import (
	"sync"
	"testing"

	"github.com/zenta-dev/zever/core/ai"
)

type eventLog struct {
	mu     sync.Mutex
	events []Event
}

func (l *eventLog) observe(ev Event) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.events = append(l.events, ev)
}

func (l *eventLog) types() []EventType {
	l.mu.Lock()
	defer l.mu.Unlock()

	out := make([]EventType, 0, len(l.events))
	for _, ev := range l.events {
		out = append(out, ev.Type)
	}

	return out
}

func TestObserve_runSequence(t *testing.T) {
	client := &scriptedAI{responses: []ai.Generation{
		{ToolCalls: []ai.ToolCall{{ID: "1", Name: "add"}}},
		{Content: "done"},
	}}

	log := &eventLog{}

	l, err := New(client, Options{
		Model:   "m",
		Tools:   []Tool{echoTool("add")},
		Observe: log.observe,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := l.Run(t.Context(), nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	want := []EventType{EventGenerate, EventToolCall, EventToolResult, EventGenerate, EventDone}
	got := log.types()

	if len(got) != len(want) {
		t.Fatalf("events = %v, want %v", got, want)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("events = %v, want %v", got, want)
		}
	}

	var toolName string
	for _, ev := range log.events {
		if ev.Type == EventToolCall {
			toolName = ev.ToolCall.Name
		}
		if ev.Type == EventToolResult && ev.Output != "ok" {
			t.Fatalf("tool result output = %q, want ok", ev.Output)
		}
	}

	if toolName != "add" {
		t.Fatalf("observed tool call = %q, want add", toolName)
	}
}

func TestObserve_nilDisables(t *testing.T) {
	client := &scriptedAI{responses: []ai.Generation{{Content: "hi"}}}

	l, err := New(client, Options{Model: "m"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := l.Run(t.Context(), nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestObserve_parallelSafe(t *testing.T) {
	var calls []ai.ToolCall
	for i := 0; i < 8; i++ {
		calls = append(calls, ai.ToolCall{ID: string(rune('a' + i)), Name: "w"})
	}

	client := &scriptedAI{responses: []ai.Generation{
		{ToolCalls: calls},
		{Content: "done"},
	}}

	log := &eventLog{}

	l, err := New(client, Options{
		Model:       "m",
		MaxParallel: 8,
		Tools:       []Tool{echoTool("w")},
		Observe:     log.observe,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := l.Run(t.Context(), nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if n := len(log.events); n != 1+8+8+1+1 {
		t.Fatalf("events = %d, want 19", n)
	}
}
