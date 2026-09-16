package config

import (
	"errors"
	"testing"
)

func TestMergeAllServices(t *testing.T) {
	t.Parallel()
	for _, svc := range knownServiceNames() {
		cfg := Default()
		if err := merge(cfg, map[string]ServiceConfig{svc: {Adapter: "x-" + svc}}); err != nil {
			t.Fatalf("%s: %v", svc, err)
		}
		adapter, _, ok := serviceRefs(cfg, svc)
		if !ok {
			t.Fatalf("%s: no refs", svc)
		}
		if *adapter != "x-"+svc {
			t.Fatalf("%s: got %q", svc, *adapter)
		}
	}
}

func TestMergeUnknownService(t *testing.T) {
	t.Parallel()
	err := merge(Default(), map[string]ServiceConfig{"nope": {Adapter: "x"}})
	var unknown *UnknownServiceError
	if !errors.As(err, &unknown) {
		t.Fatalf("want UnknownServiceError, got %v", err)
	}
	if !errors.Is(err, ErrUnknownService) {
		t.Fatalf("want ErrUnknownService match: %v", err)
	}
}

func TestMergeUnknownFieldAtoM(t *testing.T) {
	t.Parallel()
	err := merge(Default(), map[string]ServiceConfig{"db": {Options: map[string]any{"bogus": 1}}})
	if err == nil {
		t.Fatal("want error")
	}
	if !errors.Is(err, ErrUnknownField) {
		t.Fatalf("want ErrUnknownField match: %v", err)
	}
}

func TestMergeUnknownFieldNtoZ(t *testing.T) {
	t.Parallel()
	err := merge(Default(), map[string]ServiceConfig{"webhook": {Options: map[string]any{"bogus": 1}}})
	if err == nil {
		t.Fatal("want error")
	}
}

func TestMergeAdapterOverrideAndEmptyKeeps(t *testing.T) {
	t.Parallel()
	cfg := Default()
	if err := merge(cfg, map[string]ServiceConfig{"db": {Adapter: "postgres"}}); err != nil {
		t.Fatal(err)
	}
	if cfg.DB.Adapter != "postgres" {
		t.Fatalf("got %q", cfg.DB.Adapter)
	}
	before := cfg.Crypto.Options.Key
	if err := merge(cfg, map[string]ServiceConfig{
		"db":     {},
		"crypto": {Options: map[string]any{}},
	}); err != nil {
		t.Fatal(err)
	}
	if cfg.DB.Adapter != "postgres" {
		t.Fatal("empty adapter must keep")
	}
	if cfg.Crypto.Options.Key != before {
		t.Fatal("empty options must keep")
	}
	if err := merge(cfg, map[string]ServiceConfig{"db": {Options: map[string]any{"maxconns": 5}}}); err != nil {
		t.Fatal(err)
	}
	if cfg.DB.Options.MaxConns != 5 {
		t.Fatalf("got %d", cfg.DB.Options.MaxConns)
	}
}
