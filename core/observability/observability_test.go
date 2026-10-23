package observability

import (
	"testing"
)

func TestOptionsValidateMutualExclusion(t *testing.T) {
	t.Parallel()

	opts := validOptions()
	opts.CAFile = "/tmp/ca.pem"
	opts.Insecure = true

	if err := opts.Validate(); err == nil {
		t.Fatal("Validate() = nil, want ca_file/insecure exclusion error")
	}
}

func TestOptionsValidateUnspecifiedEndpoint(t *testing.T) {
	t.Parallel()

	opts := validOptions()
	opts.Endpoint = "0.0.0.0:4317"

	if err := opts.Validate(); err == nil {
		t.Fatal("Validate() = nil, want error for unspecified host")
	}
}

func TestRequestIDRoundtrip(t *testing.T) {
	t.Parallel()

	ctx := WithRequestID(t.Context(), "req-42")
	if got := RequestIDFromContext(ctx); got != "req-42" {
		t.Fatalf("request id = %q, want req-42", got)
	}

	if got := RequestIDFromContext(t.Context()); got != "" {
		t.Fatalf("request id = %q, want empty", got)
	}
}

func TestMapCarrier(t *testing.T) {
	t.Parallel()

	var c MapCarrier = map[string]string{}
	c.Set("k", "v")

	if got := c.Get("k"); got != "v" {
		t.Fatalf("Get() = %q, want v", got)
	}

	if len(c.Keys()) != 1 {
		t.Fatalf("Keys() len = %d, want 1", len(c.Keys()))
	}
}
