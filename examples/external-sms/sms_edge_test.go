package sms_test

import (
	"encoding/json/jsontext"
	"errors"
	"sync"
	"testing"

	sms "github.com/example/zever-sms"

	"github.com/zenta-dev/zever/config"
)

// registerOnce and errRegister make RegisterPlugin safe to call from
// concurrent tests: plugin registration is process-wide, so it must
// happen exactly once.
var (
	registerOnce sync.Once
	errRegister  error
)

// ensureRegistered runs sms.RegisterPlugin once and returns its result.
func ensureRegistered() error {
	registerOnce.Do(func() { errRegister = sms.RegisterPlugin() })
	return errRegister
}

// TestNewValidatesOptions pins the constructor boundary: empty options
// fail closed with ErrInvalidOptions, valid options open a backend.
func TestNewValidatesOptions(t *testing.T) {
	t.Parallel()

	if _, err := sms.New(sms.Options{}); !errors.Is(err, sms.ErrInvalidOptions) {
		t.Fatalf("New empty = %v, want ErrInvalidOptions", err)
	}

	svc, err := sms.New(sms.Options{From: "+15550000"})
	if err != nil {
		t.Fatalf("New valid: %v", err)
	}

	if svc == nil {
		t.Fatal("New returned nil SMS")
	}
}

// TestStubSendInvalidArgs pins the empty-boundary error path: an empty
// recipient or body must fail with ErrInvalidOptions, not panic.
func TestStubSendInvalidArgs(t *testing.T) {
	t.Parallel()

	svc, err := sms.New(sms.Options{From: "+15550000"})
	if err != nil {
		t.Fatalf("New: %v", err)
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

// TestBuildFromConfigErrorPaths covers nil config, a missing entry,
// invalid options, and corrupt options. Backend selection is code
// (BuildFromConfig calls New), so invalid options fail closed here.
func TestBuildFromConfigErrorPaths(t *testing.T) {
	t.Parallel()

	invalid := config.Default()
	invalid.Plugins = map[string]config.Service[jsontext.Value]{
		"sms": {Options: jsontext.Value(`{"from":""}`)},
	}

	tests := []struct {
		name string
		cfg  *config.Config
		want error
	}{
		{"nil config", nil, sms.ErrInvalidOptions},
		{"missing entry", config.Default(), sms.ErrInvalidOptions},
		{"invalid options", invalid, sms.ErrInvalidOptions},
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
	corrupt.Plugins = map[string]config.Service[jsontext.Value]{
		"sms": {Options: jsontext.Value(`{bad`)},
	}
	if _, err := sms.BuildFromConfig(corrupt); err == nil {
		t.Fatal("corrupt options: want error")
	}
}
