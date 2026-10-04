package sms_test

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"

	sms "github.com/example/zever-sms"
	"github.com/zenta-dev/zever/config"
)

// registerOnce and registerErr make RegisterPlugin safe to call from
// concurrent tests and benchmarks: registration is process-wide, so it must
// happen exactly once.
var (
	registerOnce sync.Once
	registerErr  error
)

// ensureRegistered runs sms.RegisterPlugin once and returns its result.
func ensureRegistered() error {
	registerOnce.Do(func() { registerErr = sms.RegisterPlugin() })
	return registerErr
}

// TestStubSendInvalidArgs pins the empty-boundary error path: an empty
// recipient or body must fail with ErrInvalidOptions, not panic.
func TestStubSendInvalidArgs(t *testing.T) {
	t.Parallel()

	if err := ensureRegistered(); err != nil {
		t.Fatalf("RegisterPlugin: %v", err)
	}
	svc, err := sms.Open(sms.Stub, sms.Options{From: "+15550000"})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	tests := []struct {
		name string
		to   string
		body string
	}{
		{"empty to", "", "hi"},
		{"empty body", "+15550001", ""},
		{"both empty", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := svc.Send(t.Context(), tc.to, tc.body); !errors.Is(err, sms.ErrInvalidOptions) {
				t.Fatalf("Send = %v, want ErrInvalidOptions", err)
			}
		})
	}
}

// TestOpenErrorPaths covers the constructor boundaries: unknown adapter and
// invalid options.
func TestOpenErrorPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		adapter sms.Adapter
		opts    sms.Options
		want    error
	}{
		{"invalid options", sms.Stub, sms.Options{}, sms.ErrInvalidOptions},
		{"unknown adapter", sms.Adapter("nope"), sms.Options{From: "x"}, sms.ErrUnknownAdapter},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := sms.Open(tc.adapter, tc.opts); !errors.Is(err, tc.want) {
				t.Fatalf("Open = %v, want %v", err, tc.want)
			}
		})
	}
}

// TestRegisterNilFactory pins the nil-factory guard.
func TestRegisterNilFactory(t *testing.T) {
	t.Parallel()

	if err := sms.Register(sms.Adapter("nil-factory"), nil); !errors.Is(err, sms.ErrNilFactory) {
		t.Fatalf("Register = %v, want ErrNilFactory", err)
	}
}

// TestParseAdapterEmpty pins the invalid-adapter boundary.
func TestParseAdapterEmpty(t *testing.T) {
	t.Parallel()

	if _, err := sms.ParseAdapter(""); !errors.Is(err, sms.ErrInvalidAdapter) {
		t.Fatalf("ParseAdapter(\"\") = %v, want ErrInvalidAdapter", err)
	}
}

// TestAdapterString pins the empty-adapter display contract.
func TestAdapterString(t *testing.T) {
	t.Parallel()

	if got := sms.Adapter("").String(); got != "unknown" {
		t.Fatalf("empty Adapter.String() = %q, want unknown", got)
	}
	if got := sms.Stub.String(); got != "stub" {
		t.Fatalf("Stub.String() = %q, want stub", got)
	}
}

// TestBuildFromConfigErrorPaths covers nil config, missing entry, corrupt
// options, and an unknown adapter.
func TestBuildFromConfigErrorPaths(t *testing.T) {
	t.Parallel()

	if err := ensureRegistered(); err != nil {
		t.Fatalf("RegisterPlugin: %v", err)
	}

	tests := []struct {
		name string
		cfg  *config.Config
		want error
	}{
		{"nil config", nil, sms.ErrInvalidOptions},
		{"missing entry", config.Default(), sms.ErrInvalidOptions},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := sms.BuildFromConfig(tc.cfg); !errors.Is(err, tc.want) {
				t.Fatalf("BuildFromConfig = %v, want %v", err, tc.want)
			}
		})
	}

	corrupt := config.Default()
	corrupt.Plugins = map[string]config.Service[json.RawMessage]{
		"sms": {Adapter: "stub", Options: json.RawMessage(`{bad`)},
	}
	if _, err := sms.BuildFromConfig(corrupt); err == nil {
		t.Fatal("corrupt options: want error")
	}

	unknown := config.Default()
	unknown.Plugins = map[string]config.Service[json.RawMessage]{
		"sms": {Adapter: "nope", Options: json.RawMessage(`{"from":"x"}`)},
	}
	if _, err := sms.BuildFromConfig(unknown); !errors.Is(err, sms.ErrUnknownAdapter) {
		t.Fatalf("unknown adapter = %v, want ErrUnknownAdapter", err)
	}
}
