package ollama

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/ai"
	"github.com/zenta-dev/zever/core/ai/aitest"
)

// TestOllamaConformance proves the ollama adapter honors the ai.AI
// contract via the shared conformance kit. The HTTP layer is faked
// (no network): /api/chat answers single-shot or NDJSON stream
// payloads by request shape, /api/embed answers two vectors.
func TestOllamaConformance(t *testing.T) {
	t.Parallel()

	aitest.Conformance(t, func(t *testing.T) ai.AI {
		t.Helper()

		a, err := NewWithOptions(Options{
			Addr:  "http://localhost:11434",
			Model: "kit-model",
			Transport: fakeTransport(func(r *http.Request) (*http.Response, error) {
				switch r.URL.Path {
				case "/api/embed":
					return jsonResp(http.StatusOK, map[string]any{
						"embeddings": [][]float32{{0.1, 0.2}, {0.3, 0.4}},
					}), nil
				case "/api/chat":
					body, _ := io.ReadAll(r.Body)
					if bytes.Contains(body, []byte(`"stream":true`)) {
						ndjson := "{\"message\":{\"content\":\"kit\"},\"done\":false}\n" +
							"{\"message\":{\"content\":\"\"},\"done\":true}\n"

						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       io.NopCloser(strings.NewReader(ndjson)),
						}, nil
					}

					return jsonResp(http.StatusOK, chatOK("kit", 0, 0)), nil
				default:
					return jsonResp(http.StatusNotFound, map[string]any{"error": "kit: unknown path"}), nil
				}
			}),
		})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = a.Close() })

		return a
	})
}
