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
