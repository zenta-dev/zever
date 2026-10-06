package agent

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/ai"
)

func mustLoop(t *testing.T, client ai.AI, opts Options) *Loop {
	t.Helper()

	l, err := New(client, opts)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return l
}

func TestEdgeTools_copyIsolation(t *testing.T) {
	t.Parallel()

	l := mustLoop(t, &scriptedAI{}, Options{Tools: []Tool{echoTool("a"), echoTool("b")}})

	got := l.Tools()
	got[0] = "mutated"
	_ = append(got, "extra")

	again := l.Tools()
	if len(again) != 2 || again[0] != "a" || again[1] != "b" {
		t.Fatalf("Tools() after mutation = %v, want [a b]", again)
	}
}

func TestEdgeNew_noTools(t *testing.T) {
	t.Parallel()

	l := mustLoop(t, &scriptedAI{}, Options{Model: "m"})
	if got := l.Tools(); len(got) != 0 {
		t.Fatalf("Tools() = %v, want empty", got)
	}

	res, err := l.Run(t.Context(), nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if res.Steps != 1 {
		t.Fatalf("Steps = %d, want 1", res.Steps)
	}
}

func TestEdgeWithDefaults(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   int
		want int
	}{
		{"zero selects default", 0, DefaultMaxSteps},
		{"negative selects default", -5, DefaultMaxSteps},
		{"positive kept", 3, 3},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := (Options{MaxSteps: tc.in}).withDefaults().MaxSteps; got != tc.want {
				t.Fatalf("withDefaults().MaxSteps = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestEdgeToolSpec_mapping(t *testing.T) {
	t.Parallel()

	params := map[string]any{"type": "object"}
	tool := Tool{Name: "search", Description: "searches", Parameters: params, Handler: echoTool("search").Handler}
	spec := tool.spec()

	if spec.Name != "search" || spec.Description != "searches" {
		t.Fatalf("spec = %+v, want name search description searches", spec)
	}
	if spec.Parameters["type"] != "object" {
		t.Fatalf("spec.Parameters = %v, want type object", spec.Parameters)
	}
}

func TestEdgeDecodeArgs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		raw     string
		wantErr bool
		wantLen int
	}{
		{"empty", "", false, 0},
		{"null", "null", false, 0},
		{"empty object", "{}", false, 0},
		{"values", `{"a":1,"b":"x"}`, false, 2},
		{"nested", `{"a":{"b":[1,2]}}`, false, 1},
		{"malformed", "{oops", true, 0},
		{"array", "[]", true, 0},
		{"scalar string", `"foo"`, true, 0},
		{"scalar number", "123", true, 0},
		{"scalar bool", "true", true, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := decodeArgs(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("decodeArgs(%q) error = nil, want error", tc.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("decodeArgs(%q) error = %v", tc.raw, err)
			}
			if len(got) != tc.wantLen {
				t.Fatalf("decodeArgs(%q) len = %d, want %d", tc.raw, len(got), tc.wantLen)
			}
		})
	}
}

func TestEdgeRun_generateError(t *testing.T) {
	t.Parallel()

	want := errors.New("provider down")
	client := &scriptedAI{errs: []error{want}}
	l := mustLoop(t, client, Options{Model: "m"})

	if _, err := l.Run(t.Context(), nil); !errors.Is(err, want) {
		t.Fatalf("Run() error = %v, want %v", err, want)
	}
}

func TestEdgeRun_usageAggregates(t *testing.T) {
	t.Parallel()

	client := &scriptedAI{responses: []ai.Generation{
		{
			ToolCalls: []ai.ToolCall{{ID: "1", Name: "add", Arguments: `{}`}},
			Usage:     ai.Usage{PromptTokens: 10, CompletionTokens: 2},
		},
		{
			Content: "done",
			Usage:   ai.Usage{PromptTokens: 20, CompletionTokens: 5},
		},
	}}
	l := mustLoop(t, client, Options{Model: "m", Tools: []Tool{echoTool("add")}})

	res, err := l.Run(t.Context(), nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if res.Usage.PromptTokens != 30 || res.Usage.CompletionTokens != 7 {
		t.Fatalf("Usage = %+v, want prompt 30 completion 7", res.Usage)
	}
	if res.Steps != 2 {
		t.Fatalf("Steps = %d, want 2", res.Steps)
	}
}

func TestEdgeRun_multipleToolCallsOneStep(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var order []string
	tracked := func(name string) Tool {
		return Tool{Name: name, Handler: func(_ context.Context, _ map[string]any) (string, error) {
			mu.Lock()
			defer mu.Unlock()
			order = append(order, name)
			return name + "-out", nil
		}}
	}
	client := &scriptedAI{responses: []ai.Generation{
		{ToolCalls: []ai.ToolCall{
			{ID: "1", Name: "a"},
			{ID: "2", Name: "b"},
		}},
		{Content: "done"},
	}}
	l := mustLoop(t, client, Options{Model: "m", Tools: []Tool{tracked("a"), tracked("b")}})

	res, err := l.Run(t.Context(), nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(res.ToolCalls) != 2 {
		t.Fatalf("ToolCalls = %+v, want 2", res.ToolCalls)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(order) != 2 || order[0] != "a" || order[1] != "b" {
		t.Fatalf("dispatch order = %v, want [a b]", order)
	}

	tools := 0
	for _, m := range res.Messages {
		if m.Role == ai.RoleTool {
			tools++
			if m.ToolCallID == "" {
				t.Error("tool turn missing ToolCallID")
			}
		}
	}
	if tools != 2 {
		t.Errorf("tool turns = %d, want 2", tools)
	}
}

func TestEdgeRun_malformedArgsSurfaced(t *testing.T) {
	t.Parallel()

	client := &scriptedAI{responses: []ai.Generation{
		{ToolCalls: []ai.ToolCall{{ID: "1", Name: "add", Arguments: "{oops"}}},
		{Content: "recovered"},
	}}
	l := mustLoop(t, client, Options{Model: "m", Tools: []Tool{echoTool("add")}})

	res, err := l.Run(t.Context(), nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if res.Content != "recovered" {
		t.Fatalf("Content = %q, want recovered", res.Content)
	}

	var saw bool
	for _, m := range res.Messages {
		if m.Role == ai.RoleTool && strings.HasPrefix(m.Content, "invalid arguments: ") {
			saw = true
		}
	}
	if !saw {
		t.Error("expected invalid-arguments tool turn")
	}
}

func TestEdgeRun_nonDestructiveSkipsConfirm(t *testing.T) {
	t.Parallel()

	called := false
	plain := Tool{Name: "read", ReadOnly: true, Handler: func(context.Context, map[string]any) (string, error) {
		return "data", nil
	}}
	client := &scriptedAI{responses: []ai.Generation{
		{ToolCalls: []ai.ToolCall{{ID: "1", Name: "read"}}},
		{Content: "done"},
	}}
	l := mustLoop(t, client, Options{
		Model: "m",
		Tools: []Tool{plain},
		Confirm: func(context.Context, ai.ToolCall) (bool, error) {
			called = true
			return false, nil
		},
	})

	if _, err := l.Run(t.Context(), nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if called {
		t.Error("Confirm ran for non-destructive tool")
	}
}

func TestEdgeRun_destructiveNilConfirmRuns(t *testing.T) {
	t.Parallel()

	ran := false
	del := Tool{Name: "del", Destructive: true, Handler: func(context.Context, map[string]any) (string, error) {
		ran = true
		return "deleted", nil
	}}
	client := &scriptedAI{responses: []ai.Generation{
		{ToolCalls: []ai.ToolCall{{ID: "1", Name: "del"}}},
		{Content: "done"},
	}}
	l := mustLoop(t, client, Options{Model: "m", Tools: []Tool{del}})

	if _, err := l.Run(t.Context(), nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !ran {
		t.Error("destructive handler did not run with nil Confirm")
	}
}

func TestEdgeRun_destructiveApproved(t *testing.T) {
	t.Parallel()

	var sawCall ai.ToolCall
	del := Tool{Name: "del", Destructive: true, Handler: func(context.Context, map[string]any) (string, error) {
		return "deleted", nil
	}}
	client := &scriptedAI{responses: []ai.Generation{
		{ToolCalls: []ai.ToolCall{{ID: "7", Name: "del", Arguments: `{"x":1}`}}},
		{Content: "done"},
	}}
	l := mustLoop(t, client, Options{
		Model: "m",
		Tools: []Tool{del},
		Confirm: func(_ context.Context, call ai.ToolCall) (bool, error) {
			sawCall = call
			return true, nil
		},
	})

	res, err := l.Run(t.Context(), nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if sawCall.ID != "7" || sawCall.Name != "del" {
		t.Fatalf("Confirm saw %+v, want ID 7 name del", sawCall)
	}

	var sawResult bool
	for _, m := range res.Messages {
		if m.Role == ai.RoleTool && m.Content == "deleted" {
			sawResult = true
		}
	}
	if !sawResult {
		t.Error("expected deleted tool turn")
	}
}

func TestEdgeRun_handlerSeesArgs(t *testing.T) {
	t.Parallel()

	var gotArgs map[string]any
	tool := Tool{Name: "add", Handler: func(_ context.Context, args map[string]any) (string, error) {
		gotArgs = args
		return "ok", nil
	}}
	client := &scriptedAI{responses: []ai.Generation{
		{ToolCalls: []ai.ToolCall{{ID: "1", Name: "add", Arguments: `{"a":1}`}}},
		{Content: "done"},
	}}
	l := mustLoop(t, client, Options{Model: "m", Tools: []Tool{tool}})

	if _, err := l.Run(t.Context(), nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if gotArgs["a"] != float64(1) {
		t.Fatalf("args = %v, want a=1", gotArgs)
	}
}

func TestEdgeRun_defaultMaxStepsBound(t *testing.T) {
	t.Parallel()

	client := &scriptedAI{}
	l := mustLoop(t, client, Options{Model: "m", Tools: []Tool{echoTool("loop")}})
	for range DefaultMaxSteps {
		client.responses = append(client.responses, ai.Generation{
			ToolCalls: []ai.ToolCall{{ID: "1", Name: "loop"}},
		})
	}

	_, err := l.Run(t.Context(), nil)
	var maxErr MaxStepsError
	if !errors.As(err, &maxErr) {
		t.Fatalf("Run() error = %v, want MaxStepsError", err)
	}
	if !errors.Is(err, ErrMaxSteps) {
		t.Fatalf("Run() error = %v, want ErrMaxSteps", err)
	}
	if maxErr.Max != DefaultMaxSteps {
		t.Fatalf("Max = %d, want %d", maxErr.Max, DefaultMaxSteps)
	}
	if client.calls != DefaultMaxSteps {
		t.Fatalf("calls = %d, want %d", client.calls, DefaultMaxSteps)
	}
}

func TestEdgeRun_deadlineExceeded(t *testing.T) {
	t.Parallel()

	l := mustLoop(t, &scriptedAI{}, Options{Model: "m"})
	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-1))
	defer cancel()

	if _, err := l.Run(ctx, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run() error = %v, want context.DeadlineExceeded", err)
	}
}

func TestEdgeRun_noSystemPromptNoSystemTurn(t *testing.T) {
	t.Parallel()

	client := &scriptedAI{responses: []ai.Generation{{Content: "ok"}}}
	l := mustLoop(t, client, Options{Model: "m"})

	if _, err := l.Run(t.Context(), []ai.Message{{Role: ai.RoleUser, Content: "hi"}}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, m := range client.requests[0] {
		if m.Role == ai.RoleSystem {
			t.Fatal("unexpected system turn without SystemPrompt")
		}
	}
}

func TestEdgeRun_inputNotMutated(t *testing.T) {
	t.Parallel()

	client := &scriptedAI{responses: []ai.Generation{{Content: "ok"}}}
	l := mustLoop(t, client, Options{Model: "m", SystemPrompt: "sys"})

	in := []ai.Message{{Role: ai.RoleUser, Content: "hi"}}
	if _, err := l.Run(t.Context(), in); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(in) != 1 || in[0].Content != "hi" {
		t.Fatalf("input mutated: %+v", in)
	}
}

// stubConcurrentAI is a goroutine-safe ai.AI for concurrency tests.
type stubConcurrentAI struct{}

func (stubConcurrentAI) Generate(context.Context, string, []ai.Message, ai.GenerateOptions) (ai.Generation, error) {
	return ai.Generation{Content: "ok"}, nil
}

func (stubConcurrentAI) Stream(context.Context, string, []ai.Message, ai.GenerateOptions) (<-chan ai.StreamChunk, error) {
	ch := make(chan ai.StreamChunk)
	close(ch)
	return ch, nil
}

func (stubConcurrentAI) Embed(context.Context, string, []string, ai.EmbedOptions) ([][]float32, error) {
	return nil, nil
}

func (stubConcurrentAI) Close() error { return nil }

func TestEdgeRun_concurrent(t *testing.T) {
	t.Parallel()

	l := mustLoop(t, stubConcurrentAI{}, Options{Model: "m"})

	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = l.Run(context.Background(), []ai.Message{{Role: ai.RoleUser, Content: "hi"}})
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("Run(%d) error = %v", i, err)
		}
	}
}

func TestEdgeGenerateStructured_generateError(t *testing.T) {
	t.Parallel()

	want := errors.New("generate down")
	client := &scriptedAI{errs: []error{want}}
	var out map[string]any

	if _, err := GenerateStructured(t.Context(), client, "m", nil, map[string]any{"type": "object"}, &out); !errors.Is(err, want) {
		t.Fatalf("GenerateStructured() error = %v, want %v", err, want)
	}
}

func TestEdgeGenerateStructured_nilOut(t *testing.T) {
	t.Parallel()

	if _, err := GenerateStructured(t.Context(), &scriptedAI{}, "m", nil, nil, nil); !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("GenerateStructured(nil out) error = %v, want ErrInvalidOptions", err)
	}
}

func TestEdgeGenerateStructured_canceled(t *testing.T) {
	t.Parallel()

	var out map[string]any
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := GenerateStructured(ctx, &scriptedAI{}, "m", nil, nil, &out); !errors.Is(err, context.Canceled) {
		t.Fatalf("GenerateStructured() error = %v, want context.Canceled", err)
	}
}

func TestEdgeGenerateStructured_exhaustedCause(t *testing.T) {
	t.Parallel()

	client := &scriptedAI{responses: []ai.Generation{
		{Content: "bad"},
		{Content: "bad"},
		{Content: "bad"},
	}}
	var out map[string]any

	_, err := GenerateStructured(t.Context(), client, "m", nil, map[string]any{"type": "object"}, &out)
	if !errors.Is(err, ErrStructuredOutput) {
		t.Fatalf("error = %v, want ErrStructuredOutput", err)
	}
	var structured StructuredOutputError
	if !errors.As(err, &structured) {
		t.Fatalf("error %T is not StructuredOutputError", err)
	}
	if structured.Cause == nil {
		t.Fatal("Cause is nil")
	}
	if !strings.Contains(err.Error(), "agent: structured output") {
		t.Fatalf("Error() = %q, want agent: prefix", err.Error())
	}
	// Unwrap joins sentinel and cause; both must match.
	joined := structured.Unwrap()
	if !errors.Is(joined, ErrStructuredOutput) {
		t.Fatalf("Unwrap() = %v, want ErrStructuredOutput", joined)
	}
	if !errors.Is(joined, structured.Cause) {
		t.Fatalf("Unwrap() = %v, want cause %v", joined, structured.Cause)
	}
	if client.calls != DefaultMaxRetries {
		t.Fatalf("calls = %d, want %d", client.calls, DefaultMaxRetries)
	}
}

func TestEdgeGenerateStructured_repromptAppends(t *testing.T) {
	t.Parallel()

	client := &scriptedAI{responses: []ai.Generation{
		{Content: "not json"},
		{Content: `{"a":1}`},
	}}
	var out struct {
		A int `json:"a"`
	}

	if _, err := GenerateStructured(t.Context(), client, "m", []ai.Message{{Role: ai.RoleUser, Content: "q"}}, nil, &out); err != nil {
		t.Fatalf("GenerateStructured() error = %v", err)
	}
	if len(client.requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(client.requests))
	}
	if len(client.requests[1]) != len(client.requests[0])+2 {
		t.Fatalf("retry request len = %d, want +2", len(client.requests[1]))
	}
}

func TestEdgeErrorTypes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want error
	}{
		{"unknown tool", UnknownToolError{Name: "x"}, ErrUnknownTool},
		{"unknown tool empty", UnknownToolError{}, ErrUnknownTool},
		{"max steps", MaxStepsError{Max: 3}, ErrMaxSteps},
		{"invalid options", InvalidOptionsError{Reason: "r"}, ErrInvalidOptions},
		{"invalid tool named", InvalidToolError{Name: "t", Reason: "r"}, ErrInvalidTool},
		{"invalid tool unnamed", InvalidToolError{Reason: "r"}, ErrInvalidTool},
		{"duplicate", DuplicateToolError{Name: "t"}, ErrInvalidTool},
		{"structured", StructuredOutputError{}, ErrStructuredOutput},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if !errors.Is(tc.err, tc.want) {
				t.Fatalf("%T error = %v, want %v", tc.err, tc.err, tc.want)
			}
			if !strings.HasPrefix(tc.err.Error(), strings.Split(tc.want.Error(), ":")[0]+":") {
				t.Fatalf("Error() = %q, want package prefix", tc.err.Error())
			}
		})
	}
}

func TestEdgeInvalidToolError_format(t *testing.T) {
	t.Parallel()

	unnamed := InvalidToolError{Reason: "name is required"}
	if strings.Contains(unnamed.Error(), ": :") {
		t.Fatalf("Error() = %q, want no empty name segment", unnamed.Error())
	}

	named := InvalidToolError{Name: "t", Reason: "handler is required"}
	if !strings.Contains(named.Error(), "t") || !strings.Contains(named.Error(), "handler is required") {
		t.Fatalf("Error() = %q, want name and reason", named.Error())
	}
}
