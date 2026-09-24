package ai

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var testSeq atomic.Int64

func freshAdapter() Adapter {
	return Adapter(1000 + int(testSeq.Add(1)))
}

type stubAI struct {
	genErr    error
	streamErr error
	embedErr  error
	closed    bool
}

func (s *stubAI) Generate(_ context.Context, _ string, _ []Message, _ GenerateOptions) (Generation, error) {
	if s.genErr != nil {
		return Generation{}, s.genErr
	}
	return Generation{Content: "ok", Usage: Usage{PromptTokens: 1, CompletionTokens: 2}, FinishReason: "stop"}, nil
}

func (s *stubAI) Stream(_ context.Context, _ string, _ []Message, _ GenerateOptions) (<-chan StreamChunk, error) {
	if s.streamErr != nil {
		return nil, s.streamErr
	}
	ch := make(chan StreamChunk, 1)
	ch <- StreamChunk{Delta: "hi", Done: true}
	close(ch)
	return ch, nil
}

func (s *stubAI) Embed(_ context.Context, _ string, inputs []string, _ EmbedOptions) ([][]float32, error) {
	if s.embedErr != nil {
		return nil, s.embedErr
	}
	out := make([][]float32, len(inputs))
	for i := range inputs {
		out[i] = []float32{0.1, 0.2}
	}
	return out, nil
}

func (s *stubAI) Close() error {
	s.closed = true
	return nil
}

func TestRegister_nilFactory(t *testing.T) {
	t.Parallel()
	a := freshAdapter()
	err := Register(a, nil)
	if !errors.Is(err, ErrNilFactory) {
		t.Fatalf("err = %v, want ErrNilFactory", err)
	}
	if !strings.Contains(err.Error(), a.String()) {
		t.Errorf("err %q missing adapter", err.Error())
	}
}

func TestRegister_duplicate(t *testing.T) {
	t.Parallel()
	a := freshAdapter()
	factory := func(Options) (AI, error) { return &stubAI{}, nil }
	if err := Register(a, factory); err != nil {
		t.Fatalf("first Register err = %v", err)
	}
	err := Register(a, factory)
	if !errors.Is(err, ErrDuplicateAdapter) {
		t.Fatalf("second Register err = %v, want ErrDuplicateAdapter", err)
	}
	var de *DuplicateAdapterError
	if !errors.As(err, &de) {
		t.Fatalf("err %T is not *DuplicateAdapterError", err)
	}
	if de.Adapter != a {
		t.Fatalf("Adapter = %v, want %v", de.Adapter, a)
	}
	// Ensure third call still duplicate.
	if err := Register(a, factory); !errors.Is(err, ErrDuplicateAdapter) {
		t.Fatalf("third dup err = %v", err)
	}
}

func TestOpen_unknownAdapter(t *testing.T) {
	t.Parallel()
	a := Adapter(9999)
	got, err := Open(a, Options{})
	if !errors.Is(err, ErrUnknownAdapter) {
		t.Fatalf("err = %v, want ErrUnknownAdapter", err)
	}
	var ue *UnknownAdapterError
	if !errors.As(err, &ue) {
		t.Fatalf("err %T is not *UnknownAdapterError", err)
	}
	if ue.Adapter != a {
		t.Fatalf("Adapter = %v, want %v", ue.Adapter, a)
	}
	if got != nil {
		t.Fatalf("got = %v, want nil", got)
	}
}

func TestOpen_factoryError_wrapped(t *testing.T) {
	t.Parallel()
	a := freshAdapter()
	sentinel := errors.New("boom")
	if err := Register(a, func(Options) (AI, error) { return nil, sentinel }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	got, err := Open(a, Options{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want sentinel", err)
	}
	if !strings.Contains(err.Error(), "ai: open") {
		t.Fatalf("err %q missing ai: open", err.Error())
	}
	if !strings.Contains(err.Error(), a.String()) {
		t.Fatalf("err %q missing adapter", err.Error())
	}
	if got != nil {
		t.Fatalf("got = %v, want nil", got)
	}
}

func TestOpen_success_GenerateStreamEmbedClose(t *testing.T) {
	t.Parallel()
	a := freshAdapter()
	stub := &stubAI{}
	if err := Register(a, func(Options) (AI, error) { return stub, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	got, err := Open(a, Options{})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	// Generate
	gen, err := got.Generate(t.Context(), "model-x", []Message{{Role: RoleUser, Content: "hi"}}, GenerateOptions{})
	if err != nil {
		t.Fatalf("Generate err = %v", err)
	}
	if gen.Content != "ok" {
		t.Errorf("Content = %q, want ok", gen.Content)
	}
	// Stream
	ch, err := got.Stream(t.Context(), "model-x", nil, GenerateOptions{})
	if err != nil {
		t.Fatalf("Stream err = %v", err)
	}
	chunk, ok := <-ch
	if !ok {
		t.Fatalf("channel closed empty")
	}
	if chunk.Delta != "hi" {
		t.Errorf("Delta = %q, want hi", chunk.Delta)
	}
	// Embed
	vecs, err := got.Embed(t.Context(), "embed-model", []string{"a", "b"}, EmbedOptions{Dimensions: 2})
	if err != nil {
		t.Fatalf("Embed err = %v", err)
	}
	if len(vecs) != 2 {
		t.Errorf("Embed len = %d, want 2", len(vecs))
	}
	if err := got.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}
}

func TestOpen_invalidOptions(t *testing.T) {
	t.Parallel()
	a := freshAdapter()
	if err := Register(a, func(Options) (AI, error) { return &stubAI{}, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	got, err := Open(a, Options{Timeout: -1})
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("err = %v, want ErrInvalidOptions", err)
	}
	var ioe *InvalidOptionsError
	if !errors.As(err, &ioe) {
		t.Fatalf("err %T is not *InvalidOptionsError", err)
	}
	if got != nil {
		t.Fatalf("got = %v, want nil", got)
	}
}

func TestOpen_invalidOptions_baseURL(t *testing.T) {
	t.Parallel()
	a := freshAdapter()
	if err := Register(a, func(Options) (AI, error) { return &stubAI{}, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	got, err := Open(a, Options{BaseURL: "http://example.com"})
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("err = %v, want ErrInvalidOptions", err)
	}
	if got != nil {
		t.Fatalf("got = %v, want nil", got)
	}
}

func TestGenerate_propagation(t *testing.T) {
	t.Parallel()
	a := freshAdapter()
	expErr := errors.New("gen fail")
	stub := &stubAI{genErr: expErr}
	if err := Register(a, func(Options) (AI, error) { return stub, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	ai, err := Open(a, Options{})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	_, err = ai.Generate(t.Context(), "m", nil, GenerateOptions{})
	if !errors.Is(err, expErr) {
		t.Fatalf("Generate err = %v, want expErr", err)
	}
	// Stream error
	stub2 := &stubAI{streamErr: expErr}
	b := freshAdapter()
	if err2 := Register(b, func(Options) (AI, error) { return stub2, nil }); err2 != nil {
		t.Fatalf("Register err = %v", err2)
	}
	ai2, err2 := Open(b, Options{})
	if err2 != nil {
		t.Fatalf("Open err = %v", err2)
	}
	_, err2 = ai2.Stream(t.Context(), "m", nil, GenerateOptions{})
	if !errors.Is(err2, expErr) {
		t.Fatalf("Stream err = %v, want expErr", err2)
	}
	// Embed error
	stub3 := &stubAI{embedErr: expErr}
	c := freshAdapter()
	if err3 := Register(c, func(Options) (AI, error) { return stub3, nil }); err3 != nil {
		t.Fatalf("Register err = %v", err3)
	}
	ai3, err3 := Open(c, Options{})
	if err3 != nil {
		t.Fatalf("Open err = %v", err3)
	}
	_, err3 = ai3.Embed(t.Context(), "m", []string{"x"}, EmbedOptions{})
	if !errors.Is(err3, expErr) {
		t.Fatalf("Embed err = %v, want expErr", err3)
	}
}

func TestConcurrent_RegisterAndOpen(t *testing.T) {
	t.Parallel()
	// Spawn concurrent Opens on same registered adapter.
	a := freshAdapter()
	if err := Register(a, func(Options) (AI, error) { return &stubAI{}, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	done := make(chan error, 20)
	for i := 0; i < 20; i++ {
		go func() {
			_, err := Open(a, Options{BaseURL: "https://example.com"})
			done <- err
		}()
	}
	for i := 0; i < 20; i++ {
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("concurrent Open err = %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout")
		}
	}
}
