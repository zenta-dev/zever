package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/ai"
)

// streamScriptAI serves scripted chunk slices, one per Stream call.
type streamScriptAI struct {
	scriptedAI
	streams [][]ai.StreamChunk
}

func (s *streamScriptAI) Stream(_ context.Context, _ string, _ []ai.Message, _ ai.GenerateOptions) (<-chan ai.StreamChunk, error) {
	ch := make(chan ai.StreamChunk, 16)

	i := s.calls
	s.calls++

	if i < len(s.streams) {
		for _, c := range s.streams[i] {
			ch <- c
		}
	}

	close(ch)

	return ch, nil
}

func drain(t *testing.T, ch <-chan StreamEvent) []StreamEvent {
	t.Helper()

	var events []StreamEvent

	for ev := range ch {
		events = append(events, ev)
	}

	return events
}

func TestRunStream_deltasThenDone(t *testing.T) {
	client := &streamScriptAI{streams: [][]ai.StreamChunk{
		{
			{Delta: "hel"},
			{Delta: "lo", Done: true, FinishReason: "stop"},
		},
	}}

	l, err := New(client, Options{Model: "m"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	events := drain(t, mustStream(t, l, nil))

	var text strings.Builder
	var done *Result

	for _, ev := range events {
		text.WriteString(ev.Delta)
		if ev.Done {
			done = ev.Result
		}
		if ev.Err != nil {
			t.Fatalf("unexpected error event: %v", ev.Err)
		}
	}

	if text.String() != "hello" {
		t.Fatalf("streamed text = %q, want hello", text.String())
	}
	if done == nil || done.Content != "hello" {
		t.Fatalf("done result = %+v, want content hello", done)
	}
}

func TestRunStream_toolCallThenAnswer(t *testing.T) {
	client := &streamScriptAI{streams: [][]ai.StreamChunk{
		{
			{ToolCallID: "1", ToolName: "add", ToolArgsDelta: `{"a":1}`},
			{Done: true, FinishReason: "tool_calls"},
		},
		{
			{Delta: "sum is 3", Done: true, FinishReason: "stop"},
		},
	}}

	l, err := New(client, Options{Model: "m", Tools: []Tool{echoTool("add")}})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	var sawCall, sawResult, done bool

	for ev := range mustStream(t, l, nil) {
		if ev.ToolCall != nil && ev.ToolCall.Name == "add" {
			sawCall = true
		}
		if ev.ToolResult != nil && ev.ToolResult.Output == "ok" {
			sawResult = true
		}
		if ev.Done {
			done = true
			if ev.Result == nil || ev.Result.Content != "sum is 3" {
				t.Fatalf("done result = %+v", ev.Result)
			}
		}
	}

	if !sawCall || !sawResult || !done {
		t.Fatalf("call=%v result=%v done=%v, want all true", sawCall, sawResult, done)
	}
}

func TestRunStream_chunkError(t *testing.T) {
	want := errors.New("stream broke")
	client := &streamScriptAI{streams: [][]ai.StreamChunk{
		{{Delta: "hi"}, {Err: want}},
	}}

	l, err := New(client, Options{Model: "m"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	var got error

	for ev := range mustStream(t, l, nil) {
		if ev.Err != nil {
			got = ev.Err
		}
	}

	if !errors.Is(got, want) {
		t.Fatalf("error event = %v, want %v", got, want)
	}
}

func TestRunStream_canceledContext(t *testing.T) {
	client := &streamScriptAI{}

	l, err := New(client, Options{Model: "m"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	var got error

	for ev := range l.RunStream(ctx, nil) {
		if ev.Err != nil {
			got = ev.Err
		}
	}

	if !errors.Is(got, context.Canceled) {
		t.Fatalf("error event = %v, want context.Canceled", got)
	}
}

func mustStream(t *testing.T, l *Loop, msgs []ai.Message) <-chan StreamEvent {
	t.Helper()

	return l.RunStream(t.Context(), msgs)
}
