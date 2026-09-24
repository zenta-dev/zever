package observability

import (
	"context"
	"errors"
	"testing"
)

type stubProvider struct{}

func (stubProvider) Tracer(string) Tracer           { return nil }
func (stubProvider) Meter(string) Metrics           { return nil }
func (stubProvider) Shutdown(context.Context) error { return nil }

func TestParseAdapterRoundtrip(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		a    Adapter
		name string
	}{
		{Noop, "noop"},
		{Stdout, "stdout"},
		{OTLP, "otlp"},
	} {
		if got := tc.a.String(); got != tc.name {
			t.Fatalf("String() = %q, want %q", got, tc.name)
		}

		got, err := ParseAdapter(tc.name)
		if err != nil {
			t.Fatalf("ParseAdapter(%q) error = %v", tc.name, err)
		}

		if got != tc.a {
			t.Fatalf("ParseAdapter(%q) = %v, want %v", tc.name, got, tc.a)
		}
	}
}

func TestParseAdapterInvalid(t *testing.T) {
	t.Parallel()

	if _, err := ParseAdapter("nope"); !errors.Is(err, ErrInvalidAdapter) {
		t.Fatalf("ParseAdapter invalid err = %v, want ErrInvalidAdapter", err)
	}
}

func TestRegisterNilAndDuplicate(t *testing.T) {
	a := Adapter(9001)
	if err := Register(a, nil); !errors.Is(err, ErrNilFactory) {
		t.Fatalf("Register nil err = %v, want ErrNilFactory", err)
	}

	stub := func(Options) (Provider, error) { return stubProvider{}, nil }
	if err := Register(a, stub); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if err := Register(a, stub); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("Register dup err = %v, want ErrDuplicate", err)
	}
}

func TestOpenUnknown(t *testing.T) {
	t.Parallel()

	if _, err := Open(Adapter(9999), Options{ServiceName: "test"}); !errors.Is(err, ErrUnknownAdapter) {
		t.Fatalf("Open unknown err = %v, want ErrUnknownAdapter", err)
	}
}

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

	ctx := WithRequestID(context.Background(), "req-42")
	if got := RequestIDFromContext(ctx); got != "req-42" {
		t.Fatalf("request id = %q, want req-42", got)
	}

	if got := RequestIDFromContext(context.Background()); got != "" {
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
