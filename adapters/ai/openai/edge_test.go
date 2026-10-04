package openai

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/core/ai"
)

// TestEdgeGenerate_concurrent proves many goroutines can share one adapter
// without racing on the SDK client.
func TestEdgeGenerate_concurrent(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	t.Cleanup(srv.Close)

	a, err := New(ai.Options{APIKey: "sk-test", Model: "gpt-4", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	t.Cleanup(func() { _ = a.Close() })

	ctx := t.Context()

	var wg sync.WaitGroup

	errs := make(chan error, 8)

	for range 8 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			if _, err := a.Generate(ctx, "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{}); err != nil {
				errs <- err
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent Generate: %v", err)
	}
}

// TestEdgeGenerate_emptyMessages covers the nil/empty message boundary: the
// mapping must not panic and must still issue a well-formed request.
func TestEdgeGenerate_emptyMessages(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	t.Cleanup(srv.Close)

	a, err := New(ai.Options{APIKey: "sk-test", Model: "gpt-4", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	t.Cleanup(func() { _ = a.Close() })

	for _, msgs := range [][]ai.Message{nil, {}} {
		if _, err := a.Generate(t.Context(), "", msgs, ai.GenerateOptions{}); err != nil {
			t.Errorf("Generate(%d messages) = %v, want nil", len(msgs), err)
		}
	}
}
