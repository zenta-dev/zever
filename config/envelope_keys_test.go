package config

import (
	"sort"
	"testing"
	"time"

	"github.com/zenta-dev/zever/shared/codec"
)

// These tests pin the JSON key names of the option maps that
// `zever config show --json` renders (see Config.RedactedServices). A struct
// field without a json tag is encoded under its Go field name, so a missing
// tag silently renames a wire key: the retry policy used to show "BaseDelay"
// and "MaxDelay" next to the snake_case keys of every other service, and the
// Go-API-only payment and webhook handles leaked as "Idempotency" and
// "Logger".

// TestRedactedServices_optionKeysAreSnakeCase pins the key style of every
// option map in the envelope: a lower_snake_case key, never a Go field name.
// It is the guard that catches an option struct that lost (or never had) its
// json tag.
func TestRedactedServices_optionKeysAreSnakeCase(t *testing.T) {
	t.Parallel()

	for name, svc := range Default().RedactedServices() {
		for key := range svc.Options {
			if !isSnakeCaseKey(key) {
				t.Errorf("service %q: option key %q is not snake_case", name, key)
			}
		}
	}
}

// TestRedactedServices_changedServiceOptionKeys pins the exact option keys
// of the services whose option structs carry a nested or a Go-API-only
// field: the outbox and resilience retry policies, the payment idempotency
// store handle, and the webhook logger handle. A renamed, added or dropped
// key fails the gate.
func TestRedactedServices_changedServiceOptionKeys(t *testing.T) {
	t.Parallel()

	services := Default().RedactedServices()

	tests := []struct {
		service string
		want    []string
	}{
		{service: "outbox", want: []string{
			"batch_size", "dedicated_pool", "dsn", "inbox_table", "lock_seconds",
			"max_attempts", "notify_channel", "poll_interval", "prefix",
			"publication", "publisher", "retention", "retry", "slot",
			"stall_readiness", "table",
		}},
		{service: "resilience", want: []string{
			"breaker", "bulkhead", "name", "redis", "retry", "timeout",
		}},
		{service: "payment", want: []string{
			"api_key", "auto_approve", "endpoint", "max_webhook_bytes",
			"sandbox", "secret_key", "webhook_secret",
		}},
		{service: "webhook", want: []string{
			"allow_private_targets", "dead_letter_topic", "max_retries",
			"queue_adapter", "queue_opts", "replay_tolerance", "timeout",
		}},
	}

	for _, tt := range tests {
		svc, ok := services[tt.service]
		if !ok {
			t.Errorf("service %q missing from RedactedServices", tt.service)

			continue
		}

		got := sortedOptionKeys(svc.Options)
		if !equalOptionKeys(got, tt.want) {
			t.Errorf("service %q option keys:\n got %v\nwant %v", tt.service, got, tt.want)
		}
	}
}

// TestRedactedServices_retryPolicyWireKeys pins the wire keys of the retry
// policy nested in the outbox and resilience options. The policy is a config
// option, so every field it exposes is a wire key: the snake_case names, not
// the Go field names.
func TestRedactedServices_retryPolicyWireKeys(t *testing.T) {
	t.Parallel()

	want := []string{
		"base_delay", "jitter", "jitter_max", "jitter_mode", "linear",
		"max_attempts", "max_delay", "multiplier",
	}

	services := Default().RedactedServices()

	for _, name := range []string{"outbox", "resilience"} {
		svc, ok := services[name]
		if !ok {
			t.Errorf("service %q missing from RedactedServices", name)

			continue
		}

		policy, ok := svc.Options["retry"].(map[string]any)
		if !ok {
			t.Errorf("service %q: retry option = %v, want a JSON object", name, svc.Options["retry"])

			continue
		}

		got := sortedOptionKeys(policy)
		if !equalOptionKeys(got, want) {
			t.Errorf("service %q: retry policy keys:\n got %v\nwant %v", name, got, want)
		}
	}
}

// TestRedactedServices_goAPIHandlesStayOffTheWire pins that the Go-API-only
// handles of the payment and webhook options never reach the envelope: the
// store handle and the logger handle are not wire values.
func TestRedactedServices_goAPIHandlesStayOffTheWire(t *testing.T) {
	t.Parallel()

	services := Default().RedactedServices()

	tests := []struct {
		service string
		key     string
	}{
		{service: "payment", key: "Idempotency"},
		{service: "webhook", key: "Logger"},
	}

	for _, tt := range tests {
		svc, ok := services[tt.service]
		if !ok {
			t.Errorf("service %q missing from RedactedServices", tt.service)

			continue
		}

		if _, ok := svc.Options[tt.key]; ok {
			t.Errorf("service %q: Go-API-only key %q leaked into the envelope", tt.service, tt.key)
		}
	}
}

// TestRedactedServices_retryPolicyRendersValues pins the rendered values of
// the retry policy: the snake_case keys carry the effective policy, with
// durations in their canonical string form.
func TestRedactedServices_retryPolicyRendersValues(t *testing.T) {
	t.Parallel()

	cfg := Default()
	cfg.Outbox.Options.Retry.BaseDelay = time.Second
	cfg.Outbox.Options.Retry.MaxDelay = 30 * time.Second
	cfg.Outbox.Options.Retry.Multiplier = 2
	cfg.Outbox.Options.Retry.Jitter = 0.2
	cfg.Outbox.Options.Retry.MaxAttempts = 3

	svc := cfg.RedactedServices()["outbox"]

	policy, ok := svc.Options["retry"].(map[string]any)
	if !ok {
		t.Fatalf("outbox: retry option = %v, want a JSON object", svc.Options["retry"])
	}

	got, err := codec.JSONCodec[map[string]any]{}.Encode(policy)
	if err != nil {
		t.Fatalf("encode retry policy: %v", err)
	}

	want := `{"base_delay":"1s","jitter":0.2,"jitter_max":"0s","jitter_mode":0,` +
		`"linear":false,"max_attempts":3,"max_delay":"30s","multiplier":2}`

	if string(got) != want {
		t.Errorf("outbox: retry policy JSON:\n got %s\nwant %s", got, want)
	}
}

// isSnakeCaseKey reports whether key is a lower_snake_case JSON key: a lower
// letter, then lower letters, digits or underscores.
func isSnakeCaseKey(key string) bool {
	if key == "" {
		return false
	}

	for i, r := range key {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
			if i == 0 {
				return false
			}
		case r == '_':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}

	return true
}

// sortedOptionKeys returns the keys of an option map in sorted order.
func sortedOptionKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
}

// equalOptionKeys reports whether got and want hold the same keys, in the
// same order.
func equalOptionKeys(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}

	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}

	return true
}
