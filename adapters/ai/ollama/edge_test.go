package ollama

import (
	"net/http"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/core/ai"
)

// TestEdgeGenerate_concurrent proves many goroutines can share one adapter
// without racing on the HTTP transport.
func TestEdgeGenerate_concurrent(t *testing.T) {
	t.Parallel()

	a := openFake(t, func(*http.Request) (*http.Response, error) {
		return jsonResp(http.StatusOK, chatOK("ok", 1, 1)), nil
	}, "m")

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

// TestEdgeRegister_resolvesAdapter covers the exported Register wiring: after
// Register the adapter resolves through the battery registry, and a repeated
// Register is tolerated (the duplicate error is discarded) without changing
// resolution.
func TestEdgeRegister_resolvesAdapter(t *testing.T) {
	// Serial: Register mutates the process-global battery registry.
	Register()
	Register()

	a, err := ai.Open(ai.Ollama, ai.Options{})
	if err != nil {
		t.Fatalf("ai.Open() after Register = %v, want nil", err)
	}

	if a == nil {
		t.Fatal("ai.Open() after Register = nil, want adapter")
	}

	if err := a.Close(); err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
}
