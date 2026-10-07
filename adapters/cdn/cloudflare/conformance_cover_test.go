package cloudflare

import (
	"testing"

	"github.com/zenta-dev/zever/core/cdn"
	"github.com/zenta-dev/zever/core/cdn/cdntest"
)

func TestCloudflareConformance(t *testing.T) {
	t.Skip("needs live Cloudflare credentials and network")

	cdntest.Conformance(t, func(t *testing.T) cdn.CDN {
		t.Helper()
		c, err := New(cdn.Options{
			APIToken: "live-token",
			ZoneID:   "live-zone",
		})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		t.Cleanup(func() { _ = c.Close(t.Context()) })
		return c
	})
}
