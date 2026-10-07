package cdn_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/zenta-dev/zever/core/cdn"
)

var cdnAdapterSeq atomic.Int64

func freshCDNAdapter() cdn.Adapter {
	return cdn.Adapter(fmt.Sprintf("test-%d", 1000+cdnAdapterSeq.Add(1)))
}

type stubCDN struct{}

func (stubCDN) Purge(context.Context, cdn.PurgeRequest) error { return nil }
func (stubCDN) Close(context.Context) error                   { return nil }
func (stubCDN) Name() string                                  { return "stub" }

var _ cdn.CDN = stubCDN{}

func TestAdapterString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		adapter cdn.Adapter
		want    string
	}{
		{name: "zero", adapter: cdn.Adapter(""), want: "unknown"},
		{name: "noop", adapter: cdn.AdapterNoop, want: "noop"},
		{name: "cloudflare", adapter: cdn.AdapterCloudflare, want: "cloudflare"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.adapter.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseAdapter(t *testing.T) {
	t.Parallel()

	t.Run("empty", func(t *testing.T) {
		t.Parallel()

		a, err := cdn.ParseAdapter("")
		if err == nil {
			t.Fatal("ParseAdapter(\"\") = nil, want ErrInvalidAdapter")
		}
		if !errors.Is(err, cdn.ErrInvalidAdapter) {
			t.Errorf("errors.Is(err, ErrInvalidAdapter) = false (err = %v)", err)
		}

		var invalidErr cdn.InvalidAdapterError
		if !errors.As(err, &invalidErr) {
			t.Fatalf("errors.As(err, InvalidAdapterError) = false (err = %T %v)", err, err)
		}
		if invalidErr.Adapter != "" {
			t.Errorf("InvalidAdapterError.Adapter = %q, want %q", invalidErr.Adapter, "")
		}
		if a != cdn.Adapter("") {
			t.Errorf("ParseAdapter(\"\") adapter = %q, want zero", a)
		}
	})

	t.Run("named", func(t *testing.T) {
		t.Parallel()

		a, err := cdn.ParseAdapter("noop")
		if err != nil {
			t.Fatalf("ParseAdapter(\"noop\") error = %v", err)
		}
		if a != cdn.AdapterNoop {
			t.Errorf("ParseAdapter(\"noop\") = %q, want %q", a, cdn.AdapterNoop)
		}
	})
}

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

func TestRegisterNilFactory(t *testing.T) {
	err := cdn.Register(freshCDNAdapter(), nil)
	if err == nil {
		t.Fatal("Register(nil) = nil, want ErrNilFactory")
	}
	if !errors.Is(err, cdn.ErrNilFactory) {
		t.Errorf("errors.Is(err, ErrNilFactory) = false (err = %v)", err)
	}
}

func TestRegisterDuplicate(t *testing.T) {
	a := freshCDNAdapter()
	stub := func(cdn.Options) (cdn.CDN, error) { return stubCDN{}, nil }

	if err := cdn.Register(a, stub); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	err := cdn.Register(a, stub)
	if err == nil {
		t.Fatal("Register(dup) = nil, want ErrDuplicateAdapter")
	}
	if !errors.Is(err, cdn.ErrDuplicateAdapter) {
		t.Errorf("errors.Is(err, ErrDuplicateAdapter) = false (err = %v)", err)
	}

	var dupErr cdn.DuplicateError
	if !errors.As(err, &dupErr) {
		t.Fatalf("errors.As(err, DuplicateError) = false (err = %T %v)", err, err)
	}
	if dupErr.Adapter != a {
		t.Errorf("DuplicateError.Adapter = %q, want %q", dupErr.Adapter, a)
	}
}

func TestOpenUnknown(t *testing.T) {
	_, err := cdn.Open(cdn.Adapter("test-9999"), cdn.Options{})
	if err == nil {
		t.Fatal("Open() = nil, want ErrUnknownAdapter")
	}
	if !errors.Is(err, cdn.ErrUnknownAdapter) {
		t.Errorf("errors.Is(err, ErrUnknownAdapter) = false (err = %v)", err)
	}

	var unkErr cdn.UnknownAdapterError
	if !errors.As(err, &unkErr) {
		t.Fatalf("errors.As(err, UnknownAdapterError) = false (err = %T %v)", err, err)
	}
	if unkErr.Adapter != cdn.Adapter("test-9999") {
		t.Errorf("UnknownAdapterError.Adapter = %q, want %q", unkErr.Adapter, "test-9999")
	}
}

func TestOpenFactoryError(t *testing.T) {
	a := freshCDNAdapter()
	sentinel := errors.New("boom")
	_ = cdn.Register(a, func(cdn.Options) (cdn.CDN, error) { return nil, sentinel })

	_, err := cdn.Open(a, cdn.Options{})
	if err == nil {
		t.Fatal("Open() = nil, want wrapped error")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("errors.Is(err, sentinel) = false (err = %v)", err)
	}
}

func TestOpenPassesOptions(t *testing.T) {
	a := freshCDNAdapter()
	wantOpts := cdn.Options{APIToken: "secret", ZoneID: "zone", BaseURL: "https://api.cloudflare.com"}
	var gotOpts cdn.Options

	if err := cdn.Register(a, func(opts cdn.Options) (cdn.CDN, error) {
		gotOpts = opts
		return stubCDN{}, nil
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	got, err := cdn.Open(a, wantOpts)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if got == nil {
		t.Fatal("Open() = nil, want CDN")
	}
	if gotOpts != wantOpts {
		t.Errorf("factory opts = %+v, want %+v", gotOpts, wantOpts)
	}
}

func TestOpenInvalidOptions(t *testing.T) {
	a := freshCDNAdapter()
	_ = cdn.Register(a, func(cdn.Options) (cdn.CDN, error) { return stubCDN{}, nil })

	_, err := cdn.Open(a, cdn.Options{BaseURL: "not-a-url"})
	if err == nil {
		t.Fatal("Open() = nil, want options error")
	}
	if errors.Is(err, cdn.ErrUnknownAdapter) {
		t.Errorf("Open() = %v, want options validation error before lookup", err)
	}
}
