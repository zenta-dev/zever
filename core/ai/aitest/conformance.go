// Package aitest provides the conformance kit third-party ai adapters run to prove backend parity.
package aitest

import (
	"context"
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/ai"
)

// Conformance verifies factory-built backends implement the ai.AI
// contract: open/register round-trip, Generate round-trip, Embed
// shape, Stream draining to Done, context cancellation, and Close.
// Each subtest takes a fresh instance from factory so cases stay
// isolated. Tests never call time.Sleep and never touch the network.
//
// Documented live-creds exemption: cloud adapters (anthropic, openai,
// gemini) need API keys plus network, so their conformance tests skip
// with a reason and the kit runs against fakes or the ollama seam in
// their own packages.
func Conformance(t *testing.T, factory func(t *testing.T) ai.AI) {
	t.Helper()

	t.Run("OpenRegister", func(t *testing.T) { conformanceOpenRegister(t) })
	t.Run("Generate", func(t *testing.T) { conformanceGenerate(t, factory) })
	t.Run("Embed", func(t *testing.T) { conformanceEmbed(t, factory) })
	t.Run("Stream", func(t *testing.T) { conformanceStream(t, factory) })
	t.Run("CanceledContext", func(t *testing.T) { conformanceCanceledContext(t, factory) })
	t.Run("Close", func(t *testing.T) { conformanceClose(t, factory) })
}

func conformanceOpenRegister(t *testing.T) {
	t.Helper()

	if _, err := ai.Open(ai.Adapter("conformance-missing-adapter"), ai.Options{}); !errors.Is(err, ai.ErrUnknownAdapter) {
		t.Fatalf("Open(missing) err = %v, want ErrUnknownAdapter", err)
	}

	probe := ai.Adapter("conformance-probe-ai")

	if err := ai.Register(probe, nil); !errors.Is(err, ai.ErrNilFactory) {
		t.Fatalf("Register(nil) err = %v, want ErrNilFactory", err)
	}

	stub := func(ai.Options) (ai.AI, error) {
		return nil, errors.New("aitest: probe factory must not run")
	}

	_ = ai.Register(probe, stub)

	if err := ai.Register(probe, stub); !errors.Is(err, ai.ErrDuplicate) {
		t.Fatalf("Register(duplicate) err = %v, want ErrDuplicate", err)
	}
}

func conformanceGenerate(t *testing.T, factory func(t *testing.T) ai.AI) {
	t.Helper()

	ctx := t.Context()
	a := factory(t)

	gen, err := a.Generate(ctx, "kit-model", []ai.Message{
		{Role: ai.RoleUser, Content: "kit ping"},
	}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if gen.Content == "" && len(gen.ToolCalls) == 0 {
		t.Error("Generate() returned empty content with no tool calls")
	}
}

func conformanceEmbed(t *testing.T, factory func(t *testing.T) ai.AI) {
	t.Helper()

	ctx := t.Context()
	a := factory(t)

	vecs, err := a.Embed(ctx, "kit-model", []string{"kit one", "kit two"}, ai.EmbedOptions{})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}

	if len(vecs) != 2 {
		t.Fatalf("Embed() returned %d vectors, want 2", len(vecs))
	}

	for i, v := range vecs {
		if len(v) == 0 {
			t.Errorf("Embed()[%d] is empty, want non-zero dimensions", i)
		}
	}
}

func conformanceStream(t *testing.T, factory func(t *testing.T) ai.AI) {
	t.Helper()

	ctx := t.Context()
	a := factory(t)

	ch, err := a.Stream(ctx, "kit-model", []ai.Message{
		{Role: ai.RoleUser, Content: "kit ping"},
	}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	if ch == nil {
		t.Fatal("Stream() channel is nil")
	}

	chunks := 0

	for chunk := range ch {
		chunks++

		if chunk.Err != nil {
			t.Fatalf("Stream chunk err = %v", chunk.Err)
		}

		if chunk.Done {
			break
		}
	}

	if chunks == 0 {
		t.Error("Stream() yielded no chunks")
	}
}

func conformanceCanceledContext(t *testing.T, factory func(t *testing.T) ai.AI) {
	t.Helper()

	ctx := t.Context()
	a := factory(t)

	canceled, cancel := context.WithCancel(ctx)
	cancel()

	if _, err := a.Generate(canceled, "kit-model", []ai.Message{
		{Role: ai.RoleUser, Content: "kit ping"},
	}, ai.GenerateOptions{}); err == nil {
		t.Error("Generate(canceled) = nil, want context error")
	}
}

func conformanceClose(t *testing.T, factory func(t *testing.T) ai.AI) {
	t.Helper()

	a := factory(t)

	if err := a.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := a.Close(); err != nil {
		t.Errorf("Close() second error = %v, want nil", err)
	}
}
