package anthropic

import (
	"testing"

	"github.com/zenta-dev/zever/core/ai"
	"github.com/zenta-dev/zever/core/ai/aitest"
)

// TestAnthropicConformance proves the anthropic adapter honors the
// ai.AI contract via the shared conformance kit.
//
// Currently skipped: the adapter needs a live Anthropic API key and
// network access. The kit's Generate/Embed/Stream assertions match
// the ollama adapter's; provider mapping coverage lives in the
// adapter's own httptest tests. Re-enable with a test API key.
func TestAnthropicConformance(t *testing.T) {
	t.Skip("needs live Anthropic API key and network")

	aitest.Conformance(t, func(t *testing.T) ai.AI {
		t.Helper()

		a, err := New(ai.Options{})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = a.Close() })

		return a
	})
}
