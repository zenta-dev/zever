package cdntest

import (
	"testing"

	"github.com/zenta-dev/zever/core/cdn"
)

func Conformance(t *testing.T, factory func(t *testing.T) cdn.CDN) {
	t.Helper()

	t.Run("PurgeByURL", func(t *testing.T) {
		t.Helper()
		c := factory(t)
		err := c.Purge(t.Context(), cdn.PurgeRequest{URLs: []string{"https://example.com/style.css"}})
		if err != nil {
			t.Fatalf("Purge by URL: %v", err)
		}
	})

	t.Run("PurgeByTag", func(t *testing.T) {
		t.Helper()
		c := factory(t)
		err := c.Purge(t.Context(), cdn.PurgeRequest{Tags: []string{"tag1"}})
		if err != nil {
			t.Fatalf("Purge by tag: %v", err)
		}
	})

	t.Run("PurgeAll", func(t *testing.T) {
		t.Helper()
		c := factory(t)
		err := c.Purge(t.Context(), cdn.PurgeRequest{All: true})
		if err != nil {
			t.Fatalf("Purge all: %v", err)
		}
	})

	t.Run("Name", func(t *testing.T) {
		t.Helper()
		c := factory(t)
		if c.Name() == "" {
			t.Error("Name() is empty")
		}
	})
}
