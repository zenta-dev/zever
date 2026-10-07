package noop

import (
	"testing"

	"github.com/zenta-dev/zever/core/cdn"
)

func TestNewAcceptsZeroOptions(t *testing.T) {
	c, err := New(cdn.Options{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	a, ok := c.(*noopAdapter)
	if !ok {
		t.Fatalf("New() type = %T, want *noopAdapter", c)
	}
	_ = a
}

func TestPurgeDiscards(t *testing.T) {
	tests := []struct {
		name string
		req  cdn.PurgeRequest
	}{
		{name: "urls", req: cdn.PurgeRequest{URLs: []string{"https://example.com/style.css"}}},
		{name: "tags", req: cdn.PurgeRequest{Tags: []string{"tag1"}}},
		{name: "all", req: cdn.PurgeRequest{All: true}},
		{name: "empty", req: cdn.PurgeRequest{}},
	}
	c, err := New(cdn.Options{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := c.Purge(t.Context(), tt.req); err != nil {
				t.Errorf("Purge() error = %v, want nil", err)
			}
		})
	}
}

func TestName(t *testing.T) {
	c, err := New(cdn.Options{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if got := c.Name(); got != "noop" {
		t.Errorf("Name() = %q, want %q", got, "noop")
	}
}

func TestClose(t *testing.T) {
	c, err := New(cdn.Options{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := c.Close(t.Context()); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}
