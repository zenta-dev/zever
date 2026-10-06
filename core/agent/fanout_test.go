package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/core/ai"
)

func TestRun_parallelPreservesOrder(t *testing.T) {
	client := &scriptedAI{responses: []ai.Generation{
		{ToolCalls: []ai.ToolCall{
			{ID: "1", Name: "a"},
			{ID: "2", Name: "b"},
			{ID: "3", Name: "c"},
		}},
		{Content: "done"},
	}}

	var mu sync.Mutex
	ran := map[string]bool{}

	tool := func(name, out string) Tool {
		return Tool{
			Name: name,
			Handler: func(context.Context, map[string]any) (string, error) {
				mu.Lock()
				ran[name] = true
				mu.Unlock()

				return out, nil
			},
		}
	}

	l, err := New(client, Options{
		Model:       "m",
		MaxParallel: 3,
		Tools:       []Tool{tool("a", "A"), tool("b", "B"), tool("c", "C")},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	res, err := l.Run(t.Context(), nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(ran) != 3 {
		t.Fatalf("ran %d tools, want 3", len(ran))
	}

	var got []string
	for _, m := range res.Messages {
		if m.Role == ai.RoleTool {
			got = append(got, m.Content)
		}
	}

	want := []string{"A", "B", "C"}
	if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", want) {
		t.Fatalf("tool outputs = %v, want %v in call order", got, want)
	}
}

func TestRun_parallelUnknownToolAborts(t *testing.T) {
	client := &scriptedAI{responses: []ai.Generation{
		{ToolCalls: []ai.ToolCall{
			{ID: "1", Name: "ok"},
			{ID: "2", Name: "nope"},
		}},
	}}

	l, err := New(client, Options{
		Model:       "m",
		MaxParallel: 2,
		Tools:       []Tool{echoTool("ok")},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := l.Run(t.Context(), nil); !errors.Is(err, ErrUnknownTool) {
		t.Fatalf("Run() error = %v, want ErrUnknownTool", err)
	}
}

func TestRun_sequentialByDefault(t *testing.T) {
	client := &scriptedAI{responses: []ai.Generation{
		{ToolCalls: []ai.ToolCall{{ID: "1", Name: "a"}}},
		{Content: "done"},
	}}

	var order []string

	var mu sync.Mutex
	l, err := New(client, Options{Model: "m", Tools: []Tool{{
		Name: "a",
		Handler: func(context.Context, map[string]any) (string, error) {
			mu.Lock()
			order = append(order, "a")
			mu.Unlock()

			return "A", nil
		},
	}}})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := l.Run(t.Context(), nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(order) != 1 {
		t.Fatalf("handler ran %d times, want 1", len(order))
	}
}
