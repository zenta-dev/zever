package anthropic

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/zenta-dev/zever/core/ai"
)

// TestEdgeGenerate_concurrent proves the adapter holds no shared mutable
// state: many goroutines can Generate through one client without racing.
func TestEdgeGenerate_concurrent(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"m","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"model":"c","stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	t.Cleanup(srv.Close)

	client := anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("k"))
	a := &adapter{client: &client, model: "m"}
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
		_, _ = w.Write([]byte(`{"id":"m","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"model":"c","stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	t.Cleanup(srv.Close)

	client := anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("k"))
	a := &adapter{client: &client, model: "m"}

	for _, msgs := range [][]ai.Message{nil, {}} {
		if _, err := a.Generate(t.Context(), "", msgs, ai.GenerateOptions{}); err != nil {
			t.Errorf("Generate(%d messages) = %v, want nil", len(msgs), err)
		}
	}
}
