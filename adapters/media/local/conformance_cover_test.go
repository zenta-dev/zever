package local

import (
	"testing"

	"github.com/zenta-dev/zever/core/media"
	"github.com/zenta-dev/zever/core/media/mediatest"
)

// TestOpenRegister proves Register wiring plus Open round-trip. It uses a
// test-only adapter name: registering media.Local itself would collide with
// example tests' strict Register in this same binary, so the kit factory
// below opens via New directly.
func TestOpenRegister(t *testing.T) {
	t.Parallel()

	const adapter = media.Adapter("kit-media-local")

	if err := media.Register(adapter, New); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	m, err := media.Open(adapter, media.Options{Root: t.TempDir()})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	defer func() { _ = m.Close() }()

	if _, err := m.Stat(t.Context(), "not-an-id!"); err == nil {
		t.Error("Stat(bad id) = nil, want invalid-id error")
	}
}

// TestConformance runs the shared media kit against local.
func TestConformance(t *testing.T) {
	t.Parallel()

	mediatest.Conformance(t, func(t *testing.T) media.Media {
		t.Helper()

		m, err := New(media.Options{Root: t.TempDir()})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = m.Close() })

		return m
	})
}
