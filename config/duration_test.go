package config

import (
	"testing"
	"time"
)

// TestRedactedServices_durationsRenderAsStrings pins the duration
// representation in the display envelope: a time.Duration option renders as
// its canonical string form ("30s"), the same form the env layer parses.
// encoding/json/v2 has no default representation for time.Duration, so
// before this the whole options object of every duration-carrying service
// degraded to an empty map and the envelope lied about the effective config.
func TestRedactedServices_durationsRenderAsStrings(t *testing.T) {
	t.Parallel()

	cfg := Default()
	cfg.DB.Options.MaxConnLifetime = 30 * time.Second
	cfg.DB.Options.LockTimeout = 90 * time.Second
	cfg.AI.Options.Timeout = 5 * time.Minute

	services := cfg.RedactedServices()

	tests := []struct {
		service string
		key     string
		want    string
	}{
		{service: "db", key: "max_conn_lifetime", want: "30s"},
		{service: "db", key: "lock_timeout", want: "1m30s"},
		{service: "ai", key: "timeout", want: "5m0s"},
	}

	for _, tt := range tests {
		svc, ok := services[tt.service]
		if !ok {
			t.Errorf("service %q missing from RedactedServices", tt.service)

			continue
		}

		got, ok := svc.Options[tt.key]
		if !ok {
			t.Errorf("%s: key %q missing", tt.service, tt.key)

			continue
		}

		if got != tt.want {
			t.Errorf("%s: %s = %v, want %q", tt.service, tt.key, got, tt.want)
		}
	}
}

// TestRedactedServices_durationServicesKeepOptions pins that a
// duration-carrying service still reports its non-duration options: the
// duration fields must not cost the whole options object.
func TestRedactedServices_durationServicesKeepOptions(t *testing.T) {
	t.Parallel()

	services := Default().RedactedServices()

	svc, ok := services["db"]
	if !ok {
		t.Fatal("service \"db\" missing from RedactedServices")
	}

	for _, key := range []string{"dsn", "max_conns", "max_conn_lifetime", "path"} {
		if _, ok := svc.Options[key]; !ok {
			t.Errorf("db: key %q missing from the options map", key)
		}
	}
}

// TestRedactedServices_durationServiceStillRedactsSecrets pins that the
// duration rendering does not weaken redaction: a secret on a
// duration-carrying service is still replaced by RedactedValue.
func TestRedactedServices_durationServiceStillRedactsSecrets(t *testing.T) {
	t.Parallel()

	cfg := Default()
	cfg.DB.Options.DSN = "postgres://user:s3cret@localhost/db?sslmode=disable"
	cfg.DB.Options.MaxConnLifetime = 30 * time.Second

	svc := cfg.RedactedServices()["db"]

	if got := svc.Options["dsn"]; got != RedactedValue {
		t.Errorf("db: dsn = %v, want %q", got, RedactedValue)
	}

	if got := svc.Options["max_conn_lifetime"]; got != "30s" {
		t.Errorf("db: max_conn_lifetime = %v, want %q", got, "30s")
	}
}
