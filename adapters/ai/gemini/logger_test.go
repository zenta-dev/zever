package gemini

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/ai"
)

func TestNew_UsesInjectedLogger(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	_, err := New(ai.Options{
		APIKey:             "k",
		AllowPrivate:       true,
		InsecureSkipVerify: true,
		Logger:             slog.New(slog.NewTextHandler(&buf, nil)),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	out := buf.String()
	for _, want := range []string{"allow_private enabled", "insecure_skip_verify enabled"} {
		if !strings.Contains(out, want) {
			t.Errorf("injected logger output %q missing %q", out, want)
		}
	}
}
