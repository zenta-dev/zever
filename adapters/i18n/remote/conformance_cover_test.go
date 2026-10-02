package remote

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zenta-dev/zever/core/i18n"
	"github.com/zenta-dev/zever/core/i18n/i18ntest"
)

// TestRemoteConformance proves the remote adapter honors the i18n.I18n
// contract via the shared conformance kit. The translation service
// is faked over loopback httptest (no external network): en/hello
// renders the kit fixture, unknown keys miss so the adapter reports
// ErrKeyNotFound, and /locales lists en.
func TestRemoteConformance(t *testing.T) {
	t.Parallel()

	i18ntest.Conformance(t, func(t *testing.T) i18n.I18n {
		t.Helper()

		mux := http.NewServeMux()
		mux.HandleFunc("/translate", func(w http.ResponseWriter, r *http.Request) {
			var req struct {
				Locale string            `json:"locale"`
				Key    string            `json:"key"`
				Args   map[string]string `json:"args"`
			}

			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)

				return
			}

			out := map[string]string{}
			if req.Locale == "en" && req.Key == "hello" {
				name := req.Args["name"]
				if name == "" {
					name = "Ada"
				}

				out[req.Key] = "Hello, " + name + "!"
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"translations": out})
		})
		mux.HandleFunc("/locales", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"locales": []string{"en"}})
		})

		srv := httptest.NewServer(mux)
		t.Cleanup(srv.Close)

		b, err := New(i18n.Options{Remote: i18n.RemoteOptions{
			Endpoint:      srv.URL,
			AllowInsecure: true,
		}})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = b.Close() })

		return b
	})
}
