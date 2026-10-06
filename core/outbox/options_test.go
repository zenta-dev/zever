package outbox_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/outbox"
)

func TestOptionsValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		opts    outbox.Options
		wantErr bool
	}{
		{"zero valid", outbox.Options{}, false},
		{"defaults valid", outbox.Default(), false},
		{"bad table", outbox.Options{Table: "no-dashes!"}, true},
		{"bad inbox table", outbox.Options{InboxTable: "1bad"}, true},
		{"negative poll", outbox.Options{PollInterval: -time.Second}, true},
		{"negative batch", outbox.Options{BatchSize: -1}, true},
		{"negative max attempts", outbox.Options{MaxAttempts: -1}, true},
		{"negative retention", outbox.Options{Retention: -time.Hour}, true},
		{"negative lock", outbox.Options{LockSeconds: -1}, true},
		{"bad publisher", outbox.Options{Publisher: "kafka"}, true},
		{"eventbus publisher", outbox.Options{Publisher: outbox.PublisherEventBus}, false},
		{"queue publisher", outbox.Options{Publisher: outbox.PublisherQueue}, false},
	}

	for _, tc := range tests {
		err := tc.opts.Validate()
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: Validate() = %v, wantErr %v", tc.name, err, tc.wantErr)
		}

		if tc.wantErr && !errors.Is(err, outbox.ErrInvalidOptions) {
			t.Errorf("%s: err = %v, want ErrInvalidOptions", tc.name, err)
		}
	}
}

func TestDefaultAppliesAdapterDefaults(t *testing.T) {
	t.Parallel()

	got := outbox.Default()

	if got.Table != outbox.DefaultTable {
		t.Errorf("Table = %q, want %q", got.Table, outbox.DefaultTable)
	}

	if got.InboxTable != outbox.DefaultInboxTable {
		t.Errorf("InboxTable = %q, want %q", got.InboxTable, outbox.DefaultInboxTable)
	}

	if got.PollInterval != outbox.DefaultPollInterval {
		t.Errorf("PollInterval = %v, want %v", got.PollInterval, outbox.DefaultPollInterval)
	}

	if got.BatchSize != outbox.DefaultBatchSize {
		t.Errorf("BatchSize = %d, want %d", got.BatchSize, outbox.DefaultBatchSize)
	}

	if got.MaxAttempts != outbox.DefaultMaxAttempts {
		t.Errorf("MaxAttempts = %d, want %d", got.MaxAttempts, outbox.DefaultMaxAttempts)
	}

	if got.Retention != outbox.DefaultRetention {
		t.Errorf("Retention = %v, want %v", got.Retention, outbox.DefaultRetention)
	}

	if got.LockSeconds != outbox.DefaultLockSeconds {
		t.Errorf("LockSeconds = %d, want %d", got.LockSeconds, outbox.DefaultLockSeconds)
	}
}

// TestStallReadinessIsOptIn pins the default: an existing config must never
// start gating readiness on the relay, and the flag still has to carry the
// shared snake_case serialization keys the config layer decodes.
func TestStallReadinessIsOptIn(t *testing.T) {
	t.Parallel()

	if outbox.Default().StallReadiness {
		t.Error("Default().StallReadiness = true, want false")
	}

	// Options is not JSON-encodable as a whole (Retry carries a func), so the
	// key round-trips are proved through the tags and through decoding a
	// one-field document.
	field, ok := reflect.TypeFor[outbox.Options]().FieldByName("StallReadiness")
	if !ok {
		t.Fatal("Options has no StallReadiness field")
	}

	for _, tag := range []string{"json", "toml", "yaml"} {
		if want := "stall_readiness"; field.Tag.Get(tag) != want {
			t.Errorf("StallReadiness %s tag = %q, want %q", tag, field.Tag.Get(tag), want)
		}
	}

	var decoded outbox.Options
	if err := json.Unmarshal([]byte(`{"stall_readiness":true}`), &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if !decoded.StallReadiness {
		t.Error("stall_readiness did not decode, want true")
	}
}
