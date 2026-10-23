package cdn_test

import (
	"context"
	"testing"

	"github.com/zenta-dev/zever/core/cdn"
)

type stubCDN struct{}

func (stubCDN) Purge(context.Context, cdn.PurgeRequest) error { return nil }
func (stubCDN) Close(context.Context) error                   { return nil }
func (stubCDN) Name() string                                  { return "stub" }

var _ cdn.CDN = stubCDN{}

func TestOptionsValidate(t *testing.T) {
	t.Parallel()

	t.Run("zero", func(t *testing.T) {
		t.Parallel()
		if err := (cdn.Options{}).Validate(); err != nil {
			t.Errorf("zero Options Validate() error = %v", err)
		}
	})

	t.Run("valid_urls", func(t *testing.T) {
		t.Parallel()

		for _, baseURL := range []string{"https://api.cloudflare.com", "http://localhost:8080"} {
			if err := (cdn.Options{BaseURL: baseURL}).Validate(); err != nil {
				t.Errorf("BaseURL %q Validate() error = %v", baseURL, err)
			}
		}
	})

	t.Run("invalid_urls", func(t *testing.T) {
		t.Parallel()

		for _, baseURL := range []string{"ftp://example.com", "example.com", "://missing-scheme"} {
			err := (cdn.Options{BaseURL: baseURL}).Validate()
			if err == nil {
				t.Errorf("BaseURL %q Validate() = nil, want error", baseURL)
			}
		}
	})
}
