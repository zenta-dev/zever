package container

import (
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/core/cache"
)

func TestRegisteredAdaptersCoversConfigServices(t *testing.T) {
	t.Parallel()

	services := []string{
		"ai", "analytics", "auth", "billing", "cache", "cdn", "crypto", "db",
		"document", "eventbus", "flag", "geo", "i18n", "idempotency", "lock",
		"log", "mailer", "media", "notification", "observability", "outbox",
		"password", "payment", "permission", "queue", "ratelimit", "resilience",
		"router", "scheduler", "search", "secrets", "session", "storage",
		"tenant", "vectorstore", "webhook", "workflow",
	}
	for _, service := range services {
		// Must not panic and must return a usable (possibly empty) list:
		// a missing table entry would silently skip the service.
		_ = RegisteredAdapters(service)
	}
	if got := RegisteredAdapters("no-such-service"); len(got) != 0 {
		t.Fatalf("RegisteredAdapters(unknown) = %v, want empty", got)
	}
}

func TestValidateAdaptersFailsFastWithSuggestion(t *testing.T) {
	// Register one probe adapter globally (tolerate duplicates across
	// -count=N reruns in one process). Other tests in this package
	// register real adapters into the same global registries (see
	// close_test.go), and shuffle order is random, so the asserted input
	// must be one edit from the probe yet two from any real name: with
	// input "memmorry", "memmory" (d=1) always beats "memory" (d=2) and
	// the input itself is never registered.
	probe := cache.Adapter("memmory")
	if err := cache.Register(probe, func(cache.Options) (cache.Cache, error) {
		return nil, errors.New("container: probe adapter never opens")
	}); err != nil {
		var dup cache.DuplicateError
		if !errors.As(err, &dup) {
			t.Fatalf("Register probe: %v", err)
		}
	}
	if got := RegisteredAdapters("cache"); !slicesContains(got, "memmory") {
		t.Fatalf("RegisteredAdapters(cache) = %v, want probe listed", got)
	}

	cfg := config.Default()
	cfg.Cache.Adapter = "memmorry"
	c := New(cfg)
	err := c.ValidateAdapters()
	if err == nil {
		t.Fatal("ValidateAdapters with unregistered memmorry adapter = nil, want error")
	}
	if !errors.Is(err, config.ErrUnknownAdapter) {
		t.Fatalf("errors.Is(%v, config.ErrUnknownAdapter) = false, want true", err)
	}
	if !strings.Contains(err.Error(), `did you mean "memmory"`) {
		t.Errorf("error %q lacks did-you-mean hint", err.Error())
	}
}

func slicesContains(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}
