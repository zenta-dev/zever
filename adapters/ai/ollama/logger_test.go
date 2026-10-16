package ollama

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

	_, err := NewWithOptions(Options{
		AllowPrivate:       true,
		InsecureSkipVerify: true,
		Logger:             slog.New(slog.NewTextHandler(&buf, nil)),
	})
	if err != nil {
		t.Fatalf("NewWithOptions() error = %v", err)
	}

	out := buf.String()
	for _, want := range []string{"allow_private enabled", "insecure_skip_verify enabled"} {
		if !strings.Contains(out, want) {
			t.Errorf("injected logger output %q missing %q", out, want)
		}
	}
}

func TestNew_PassesLoggerThrough(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	_, err := New(ai.Options{
		AllowPrivate: true,
		Logger:       slog.New(slog.NewTextHandler(&buf, nil)),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if !strings.Contains(buf.String(), "allow_private enabled") {
		t.Errorf("logger output %q missing allow_private notice", buf.String())
	}
}
