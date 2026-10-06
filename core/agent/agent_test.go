package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/ai"
)

type scriptedAI struct {
	responses []ai.Generation
	errs      []error
	calls     int
	requests  [][]ai.Message
	opts      []ai.GenerateOptions
}

func (s *scriptedAI) Generate(_ context.Context, _ string, messages []ai.Message, opts ai.GenerateOptions) (ai.Generation, error) {
	s.requests = append(s.requests, append([]ai.Message(nil), messages...))
	s.opts = append(s.opts, opts)

	i := s.calls
	s.calls++

	if i < len(s.errs) && s.errs[i] != nil {
		return ai.Generation{}, s.errs[i]
	}
	if i < len(s.responses) {
		return s.responses[i], nil
	}

	return ai.Generation{Content: "done", FinishReason: "stop"}, nil
}

func (s *scriptedAI) Stream(context.Context, string, []ai.Message, ai.GenerateOptions) (<-chan ai.StreamChunk, error) {
	ch := make(chan ai.StreamChunk)
	close(ch)
	return ch, nil
}

func (s *scriptedAI) Embed(context.Context, string, []string, ai.EmbedOptions) ([][]float32, error) {
	return nil, nil
}

func (s *scriptedAI) Close() error { return nil }

func echoTool(name string) Tool {
	return Tool{
		Name: name,
		Handler: func(_ context.Context, args map[string]any) (string, error) {
			return "ok", nil
		},
	}
}

func TestNew_nilClient(t *testing.T) {
	t.Parallel()

	if _, err := New(nil, Options{}); !errors.Is(err, ErrNilClient) {
		t.Fatalf("New(nil) error = %v, want ErrNilClient", err)
	}
}

func TestNew_invalidTools(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		tools []Tool
		want  error
	}{
		{"empty name", []Tool{{Handler: func(context.Context, map[string]any) (string, error) { return "", nil }}}, ErrInvalidTool},
		{"nil handler", []Tool{{Name: "x"}}, ErrInvalidTool},
		{"duplicate", []Tool{echoTool("dup"), echoTool("dup")}, ErrInvalidTool},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if _, err := New(&scriptedAI{}, Options{Tools: tc.tools}); !errors.Is(err, tc.want) {
				t.Fatalf("New() error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestNew_toolOrder(t *testing.T) {
	t.Parallel()

	l, err := New(&scriptedAI{}, Options{Tools: []Tool{echoTool("a"), echoTool("b")}})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	got := l.Tools()
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("Tools() = %v, want [a b]", got)
	}
}

func TestRun_noToolCalls(t *testing.T) {
	t.Parallel()

	client := &scriptedAI{responses: []ai.Generation{{Content: "hello"}}}
	l, err := New(client, Options{Model: "m"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	res, err := l.Run(t.Context(), []ai.Message{{Role: ai.RoleUser, Content: "hi"}})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if res.Content != "hello" || res.Steps != 1 {
		t.Fatalf("Run() = %+v, want content hello steps 1", res)
	}
	if len(res.Messages) != 2 {
		t.Errorf("Messages len = %d, want 2", len(res.Messages))
	}
}

func TestRun_toolCallThenAnswer(t *testing.T) {
	t.Parallel()

	client := &scriptedAI{responses: []ai.Generation{
		{ToolCalls: []ai.ToolCall{{ID: "1", Name: "add", Arguments: `{"a":1}`}}},
		{Content: "sum is 3"},
	}}
	l, err := New(client, Options{Model: "m", Tools: []Tool{echoTool("add")}})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	res, err := l.Run(t.Context(), []ai.Message{{Role: ai.RoleUser, Content: "add"}})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if res.Content != "sum is 3" || res.Steps != 2 {
		t.Fatalf("Run() = %+v, want content sum is 3 steps 2", res)
	}
	if len(res.ToolCalls) != 1 || res.ToolCalls[0].Name != "add" {
		t.Errorf("ToolCalls = %+v, want one add", res.ToolCalls)
	}

	var sawTool bool
	for _, m := range res.Messages {
		if m.Role == ai.RoleTool && m.Content == "ok" {
			sawTool = true
		}
	}
	if !sawTool {
		t.Error("expected a tool turn carrying the handler result")
	}
}

func TestRun_unknownToolFailsClosed(t *testing.T) {
	t.Parallel()

	client := &scriptedAI{responses: []ai.Generation{
		{ToolCalls: []ai.ToolCall{{ID: "1", Name: "nope"}}},
	}}
	l, err := New(client, Options{Model: "m"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := l.Run(t.Context(), nil); !errors.Is(err, ErrUnknownTool) {
		t.Fatalf("Run() error = %v, want ErrUnknownTool", err)
	}
}

func TestRun_handlerErrorReportedToModel(t *testing.T) {
	t.Parallel()

	boom := Tool{Name: "boom", Handler: func(context.Context, map[string]any) (string, error) {
		return "", errors.New("kaboom")
	}}
	client := &scriptedAI{responses: []ai.Generation{
		{ToolCalls: []ai.ToolCall{{ID: "1", Name: "boom"}}},
		{Content: "recovered"},
	}}
	l, err := New(client, Options{Model: "m", Tools: []Tool{boom}})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	res, err := l.Run(t.Context(), nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if res.Content != "recovered" {
		t.Fatalf("Run() content = %q, want recovered", res.Content)
	}

	var sawErr bool
	for _, m := range res.Messages {
		if m.Role == ai.RoleTool && m.Content == "error: kaboom" {
			sawErr = true
		}
	}
	if !sawErr {
		t.Error("expected handler error surfaced as a tool turn")
	}
}

func TestRun_destructiveDeclined(t *testing.T) {
	t.Parallel()

	del := Tool{Name: "del", Destructive: true, Handler: func(context.Context, map[string]any) (string, error) {
		t.Error("handler ran despite decline")
		return "ran", nil
	}}
	client := &scriptedAI{responses: []ai.Generation{
		{ToolCalls: []ai.ToolCall{{ID: "1", Name: "del"}}},
		{Content: "ok"},
	}}
	l, err := New(client, Options{
		Model:   "m",
		Tools:   []Tool{del},
		Confirm: func(context.Context, ai.ToolCall) (bool, error) { return false, nil },
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	res, err := l.Run(t.Context(), nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	var declined bool
	for _, m := range res.Messages {
		if m.Role == ai.RoleTool && m.Content == "declined by user" {
			declined = true
		}
	}
	if !declined {
		t.Error("expected declined tool turn")
	}
}

func TestRun_confirmError(t *testing.T) {
	t.Parallel()

	want := errors.New("confirm failed")
	del := Tool{Name: "del", Destructive: true, Handler: func(context.Context, map[string]any) (string, error) {
		return "ran", nil
	}}
	client := &scriptedAI{responses: []ai.Generation{
		{ToolCalls: []ai.ToolCall{{ID: "1", Name: "del"}}},
	}}
	l, err := New(client, Options{
		Model:   "m",
		Tools:   []Tool{del},
		Confirm: func(context.Context, ai.ToolCall) (bool, error) { return false, want },
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := l.Run(t.Context(), nil); !errors.Is(err, want) {
		t.Fatalf("Run() error = %v, want %v", err, want)
	}
}

func TestRun_maxSteps(t *testing.T) {
	t.Parallel()

	client := &scriptedAI{}
	l, err := New(client, Options{Model: "m", MaxSteps: 1, Tools: []Tool{echoTool("loop")}})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	client.responses = []ai.Generation{
		{ToolCalls: []ai.ToolCall{{ID: "1", Name: "loop"}}},
	}

	if _, err := l.Run(t.Context(), nil); !errors.Is(err, ErrMaxSteps) {
		t.Fatalf("Run() error = %v, want ErrMaxSteps", err)
	}
}

func TestRun_canceledContext(t *testing.T) {
	t.Parallel()

	l, err := New(&scriptedAI{}, Options{Model: "m"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := l.Run(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run(canceled) error = %v, want context.Canceled", err)
	}
}

func TestRun_systemPromptPrepended(t *testing.T) {
	t.Parallel()

	client := &scriptedAI{responses: []ai.Generation{{Content: "ok"}}}
	l, err := New(client, Options{Model: "m", SystemPrompt: "be nice"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := l.Run(t.Context(), []ai.Message{{Role: ai.RoleUser, Content: "hi"}}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(client.requests) == 0 || client.requests[0][0].Role != ai.RoleSystem {
		t.Fatalf("first request = %+v, want system prompt first", client.requests)
	}
}

func TestGenerateStructured_success(t *testing.T) {
	t.Parallel()

	client := &scriptedAI{responses: []ai.Generation{{Content: `{"name":"zever"}`}}}
	var out struct {
		Name string `json:"name"`
	}

	if _, err := GenerateStructured(t.Context(), client, "m", nil, map[string]any{"type": "object"}, &out); err != nil {
		t.Fatalf("GenerateStructured() error = %v", err)
	}
	if out.Name != "zever" {
		t.Fatalf("out.Name = %q, want zever", out.Name)
	}
}

func TestGenerateStructured_retriesThenSucceeds(t *testing.T) {
	t.Parallel()

	client := &scriptedAI{responses: []ai.Generation{
		{Content: "not json"},
		{Content: `{"name":"zever"}`},
	}}
	var out struct {
		Name string `json:"name"`
	}

	if _, err := GenerateStructured(t.Context(), client, "m", nil, map[string]any{"type": "object"}, &out); err != nil {
		t.Fatalf("GenerateStructured() error = %v", err)
	}
	if out.Name != "zever" {
		t.Fatalf("out.Name = %q, want zever", out.Name)
	}
	if client.calls != 2 {
		t.Errorf("calls = %d, want 2", client.calls)
	}
}

func TestGenerateStructured_exhausted(t *testing.T) {
	t.Parallel()

	client := &scriptedAI{responses: []ai.Generation{
		{Content: "bad"},
		{Content: "bad"},
		{Content: "bad"},
	}}
	var out map[string]any

	if _, err := GenerateStructured(t.Context(), client, "m", nil, map[string]any{"type": "object"}, &out); !errors.Is(err, ErrStructuredOutput) {
		t.Fatalf("GenerateStructured() error = %v, want ErrStructuredOutput", err)
	}
}

func TestGenerateStructured_nilClient(t *testing.T) {
	t.Parallel()

	var out map[string]any
	if _, err := GenerateStructured(t.Context(), nil, "m", nil, nil, &out); !errors.Is(err, ErrNilClient) {
		t.Fatalf("GenerateStructured(nil) error = %v, want ErrNilClient", err)
	}
}
