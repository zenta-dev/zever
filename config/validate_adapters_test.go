package config

import (
	"errors"
	"strings"
	"testing"
)

func knownStub(cfg *Config) func(string) []string {
	known := map[string][]string{}
	for _, sa := range cfg.adapters() {
		known[sa.service] = []string{sa.adapter}
	}
	return func(service string) []string { return known[service] }
}

func TestValidateAdaptersAllKnownPasses(t *testing.T) {
	t.Parallel()

	cfg := Default()
	if err := cfg.ValidateAdapters(knownStub(cfg)); err != nil {
		t.Fatalf("ValidateAdapters with everything known = %v, want nil", err)
	}
}

func TestValidateAdaptersUnknownWithSuggestion(t *testing.T) {
	t.Parallel()

	cfg := Default()
	cfg.Cache.Adapter = "memmory"
	stub := knownStub(cfg)
	inner := stub
	registered := func(service string) []string {
		if service == "cache" {
			return []string{"memory", "redis"}
		}
		return inner(service)
	}

	err := cfg.ValidateAdapters(registered)
	if err == nil {
		t.Fatal("ValidateAdapters with unknown cache adapter = nil, want error")
	}
	if !errors.Is(err, ErrUnknownAdapter) {
		t.Fatalf("errors.Is(%v, ErrUnknownAdapter) = false, want true", err)
	}
	var target *UnknownAdapterError
	if !errors.As(err, &target) {
		t.Fatalf("errors.As failed for %v", err)
	}
	if target.Service != "cache" || target.Adapter != "memmory" || target.Suggestion != "memory" {
		t.Errorf("UnknownAdapterError = %+v, want service cache adapter memmory suggestion memory", target)
	}
	if !strings.Contains(err.Error(), `did you mean "memory"`) {
		t.Errorf("error %q lacks did-you-mean hint", err.Error())
	}
}

func TestValidateAdaptersJoinsMultiple(t *testing.T) {
	t.Parallel()

	cfg := Default()
	stub := knownStub(Default())
	cfg.Cache.Adapter = "nope"
	cfg.Queue.Adapter = "nada"

	err := cfg.ValidateAdapters(stub)
	if err == nil {
		t.Fatal("ValidateAdapters with two unknown adapters = nil, want error")
	}
	if msg := err.Error(); !strings.Contains(msg, "cache") || !strings.Contains(msg, "queue") {
		t.Errorf("joined error %q does not name both services", msg)
	}
}

func TestValidateAdaptersEmptyRegistryFailsClosed(t *testing.T) {
	t.Parallel()

	cfg := Default()
	err := cfg.ValidateAdapters(func(string) []string { return nil })
	if err == nil {
		t.Fatal("ValidateAdapters with empty registry = nil, want error")
	}
	if !errors.Is(err, ErrUnknownAdapter) {
		t.Fatalf("errors.Is(%v, ErrUnknownAdapter) = false, want true", err)
	}
}

// scaffoldRegistry mimics a minimal scaffold: only db and router adapters
// are registered.
func scaffoldRegistry(service string) []string {
	switch service {
	case "db":
		return []string{"sqlite", "postgres"}
	case "router":
		return []string{"chi"}
	}
	return nil
}

func TestValidateEnabledAdaptersScaffoldPasses(t *testing.T) {
	t.Parallel()

	cfg := Default()
	cfg.DB.Adapter = "sqlite"
	cfg.Router.Adapter = "chi"

	if err := cfg.ValidateEnabledAdapters([]string{"db", "router"}, scaffoldRegistry); err != nil {
		t.Fatalf("scaffold-like config = %v, want nil (unconfigured services must be skipped)", err)
	}
}

func TestValidateEnabledAdaptersUnregisteredFailsWithHint(t *testing.T) {
	t.Parallel()

	cfg := Default()
	cfg.DB.Adapter = "sqlite"
	cfg.Router.Adapter = "chi"
	cfg.Cache.Adapter = "redsi"

	reg := func(service string) []string {
		if service == "cache" {
			return []string{"memory", "redis"}
		}
		return scaffoldRegistry(service)
	}

	err := cfg.ValidateEnabledAdapters([]string{"db", "router", "cache"}, reg)
	if err == nil {
		t.Fatal("want error for unregistered cache adapter")
	}

	var ua *UnknownAdapterError
	if !errors.As(err, &ua) || ua.Service != "cache" || ua.Suggestion != "redis" {
		t.Fatalf("err = %v, want cache UnknownAdapterError suggesting redis", err)
	}
}

func TestValidateEnabledAdaptersNonDefaultAdapterCountsAsEnabled(t *testing.T) {
	t.Parallel()

	cfg := Default()
	cfg.Cache.Adapter = "redis"

	err := cfg.ValidateEnabledAdapters(nil, func(string) []string { return []string{"memory"} })
	if err == nil || !strings.Contains(err.Error(), `"redis"`) {
		t.Fatalf("err = %v, want failure for non-default cache adapter", err)
	}
}

func TestValidateEnabledAdaptersCustomRegisteredPasses(t *testing.T) {
	t.Parallel()

	cfg := Default()
	cfg.Cache.Adapter = "my-custom"

	err := cfg.ValidateEnabledAdapters([]string{"cache"}, func(string) []string { return []string{"my-custom"} })
	if err != nil {
		t.Fatalf("custom registered adapter = %v, want nil", err)
	}
}

// TestValidateAdaptersMessageCarriesOnePrefix pins the error convention on
// the failure the doctor row prints: the message names the package exactly
// once ("config: unknown adapter ..."), never twice ("config: config:
// unknown adapter ..."), and the sentinel still matches through the joined
// error.
func TestValidateAdaptersMessageCarriesOnePrefix(t *testing.T) {
	t.Parallel()

	cfg := Default()
	cfg.Cache.Adapter = "memmory"

	stub := knownStub(cfg)
	registered := func(service string) []string {
		if service == "cache" {
			return []string{"memory"}
		}

		return stub(service)
	}

	err := cfg.ValidateAdapters(registered)
	if err == nil {
		t.Fatal("ValidateAdapters with unknown cache adapter = nil, want error")
	}

	want := `config: unknown adapter "memmory" for service "cache" (did you mean "memory"?)`
	if got := err.Error(); got != want {
		t.Errorf("error = %q, want %q", got, want)
	}

	if !errors.Is(err, ErrUnknownAdapter) {
		t.Errorf("errors.Is(%v, ErrUnknownAdapter) = false, want true", err)
	}
}

func TestUnknownAdapterErrorMessage(t *testing.T) {
	t.Parallel()

	plain := &UnknownAdapterError{Service: "cache", Adapter: "nope"}
	if got, want := plain.Error(), `config: unknown adapter "nope" for service "cache"`; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	withHint := &UnknownAdapterError{Service: "cache", Adapter: "memmory", Suggestion: "memory"}
	if got := withHint.Error(); !strings.Contains(got, `did you mean "memory"`) {
		t.Errorf("Error() = %q, want did-you-mean hint", got)
	}
	if !errors.Is(withHint, ErrUnknownAdapter) {
		t.Errorf("errors.Is(hint, ErrUnknownAdapter) = false, want true")
	}
}
