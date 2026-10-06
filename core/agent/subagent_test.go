package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/ai"
)

type stubSubAgent struct {
	content string
	err     error
	calls   int
}

func (s *stubSubAgent) Run(context.Context, []ai.Message) (Result, error) {
	s.calls++

	if s.err != nil {
		return Result{}, s.err
	}

	return Result{Content: s.content}, nil
}

func TestAsTool_delegates(t *testing.T) {
	sub := &stubSubAgent{content: "sub answer"}
	tool := AsTool("research", "Research a topic.", sub, map[string]any{"type": "object"})

	if tool.Name != "research" {
		t.Fatalf("tool name = %q, want research", tool.Name)
	}

	out, err := tool.Handler(t.Context(), map[string]any{"q": "x"})
	if err != nil {
		t.Fatalf("handler error = %v", err)
	}

	if out != "sub answer" {
		t.Fatalf("output = %q, want sub answer", out)
	}

	if sub.calls != 1 {
		t.Fatalf("sub ran %d times, want 1", sub.calls)
	}
}

func TestAsTool_subFailurePropagates(t *testing.T) {
	want := errors.New("sub down")
	tool := AsTool("research", "Research.", &stubSubAgent{err: want}, nil)

	if _, err := tool.Handler(t.Context(), nil); !errors.Is(err, want) {
		t.Fatalf("handler error = %v, want %v", err, want)
	}
}

func TestAsTool_inParentLoop(t *testing.T) {
	client := &scriptedAI{responses: []ai.Generation{
		{ToolCalls: []ai.ToolCall{{ID: "1", Name: "research", Arguments: `{"q":"x"}`}}},
		{Content: "final"},
	}}

	sub := &stubSubAgent{content: "found it"}

	l, err := New(client, Options{
		Model: "m",
		Tools: []Tool{AsTool("research", "Research.", sub, nil)},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	res, err := l.Run(t.Context(), nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if res.Content != "final" {
		t.Fatalf("content = %q, want final", res.Content)
	}
}
