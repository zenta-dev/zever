package noop

import (
	"testing"

	"github.com/zenta-dev/zever/core/cdn"
	"github.com/zenta-dev/zever/core/cdn/cdntest"
)

func TestNoopConformance(t *testing.T) {
	cdntest.Conformance(t, func(t *testing.T) cdn.CDN {
		t.Helper()
		c, err := New(cdn.Options{})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		t.Cleanup(func() { _ = c.Close(t.Context()) })
		return c
	})
}
