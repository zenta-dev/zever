package lock

import (
	"testing"
	"time"
)

func TestParseOptions_nil_returnsDefaults(t *testing.T) {
	t.Parallel()

	o, err := ParseOptions(nil)
	if err != nil {
		t.Fatalf("ParseOptions(nil) error = %v", err)
	}

	if o.Addr != "localhost:6379" {
		t.Errorf("Addr = %q, want localhost:6379", o.Addr)
	}

	if o.Prefix != "lock:" {
		t.Errorf("Prefix = %q, want lock:", o.Prefix)
	}

	if o.TTL != DefaultTTL {
		t.Errorf("TTL = %v, want %v", o.TTL, DefaultTTL)
	}

	if o.RetryInterval != DefaultRetryInterval {
		t.Errorf("RetryInterval = %v, want %v", o.RetryInterval, DefaultRetryInterval)
	}
}

func TestParseOptions_overrides(t *testing.T) {
	t.Parallel()

	o, err := ParseOptions(map[string]any{
		"url":            "redis://localhost:6379/0",
		"addr":           "redis:6380",
		"password":       "secret",
		"db":             5,
		"prefix":         "x:",
		"ttl":            "45s",
		"retry_interval": "5ms",
	})
	if err != nil {
		t.Fatalf("ParseOptions() error = %v", err)
	}

	if o.URL != "redis://localhost:6379/0" {
		t.Errorf("URL = %q", o.URL)
	}

	if o.Addr != "redis:6380" {
		t.Errorf("Addr = %q, want redis:6380", o.Addr)
	}

	if o.Password != "secret" {
		t.Errorf("Password = %q", o.Password)
	}

	if o.DB != 5 {
		t.Errorf("DB = %d, want 5", o.DB)
	}

	if o.Prefix != "x:" {
		t.Errorf("Prefix = %q, want x:", o.Prefix)
	}

	if o.TTL != 45*time.Second {
		t.Errorf("TTL = %v, want 45s", o.TTL)
	}

	if o.RetryInterval != 5*time.Millisecond {
		t.Errorf("RetryInterval = %v, want 5ms", o.RetryInterval)
	}
}

func TestParseOptions_durationTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value any
		want  time.Duration
	}{
		{name: "duration", value: 2 * time.Minute, want: 2 * time.Minute},
		{name: "int seconds", value: 7, want: 7 * time.Second},
		{name: "int64 seconds", value: int64(8), want: 8 * time.Second},
		{name: "float seconds", value: 1.5, want: time.Duration(1.5 * float64(time.Second))},
		{name: "string", value: "10s", want: 10 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			o, err := ParseOptions(map[string]any{"ttl": tt.value})
			if err != nil {
				t.Fatalf("ParseOptions() error = %v", err)
			}

			if o.TTL != tt.want {
				t.Errorf("TTL = %v, want %v", o.TTL, tt.want)
			}
		})
	}
}

func TestParseOptions_intTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value any
		want  int
	}{
		{name: "int", value: 3, want: 3},
		{name: "int64", value: int64(4), want: 4},
		{name: "float64", value: float64(5), want: 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			o, err := ParseOptions(map[string]any{"db": tt.value})
			if err != nil {
				t.Fatalf("ParseOptions() error = %v", err)
			}

			if o.DB != tt.want {
				t.Errorf("DB = %d, want %d", o.DB, tt.want)
			}
		})
	}
}

func TestParseOptions_tls(t *testing.T) {
	t.Parallel()

	o, err := ParseOptions(map[string]any{"tls": true})
	if err != nil {
		t.Fatalf("ParseOptions(tls) error = %v", err)
	}

	if !o.TLS {
		t.Error("TLS = false, want true")
	}

	if _, err := ParseOptions(map[string]any{"tls": "yes"}); err == nil {
		t.Error("ParseOptions(tls=string) = nil, want error")
	}
}

func TestParseOptions_unknownKey_returnsError(t *testing.T) {
	t.Parallel()

	if _, err := ParseOptions(map[string]any{"nope": "x"}); err == nil {
		t.Fatal("ParseOptions(unknown) = nil, want error")
	}
}

func TestParseOptions_wrongType_returnsError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		key   string
		value any
	}{
		{name: "url int", key: "url", value: 42},
		{name: "addr int", key: "addr", value: 42},
		{name: "password int", key: "password", value: 42},
		{name: "db string", key: "db", value: "nope"},
		{name: "prefix int", key: "prefix", value: 42},
		{name: "ttl bool", key: "ttl", value: true},
		{name: "retry bool", key: "retry_interval", value: true},
		{name: "ttl bad duration", key: "ttl", value: "not-a-duration"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := ParseOptions(map[string]any{tt.key: tt.value}); err == nil {
				t.Fatalf("ParseOptions(%q=%T) = nil, want error", tt.key, tt.value)
			}
		})
	}
}

func TestOptionsValidate_zero_returnsNil(t *testing.T) {
	t.Parallel()

	if err := (Options{}).Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestDefaults_values(t *testing.T) {
	t.Parallel()

	if DefaultTTL != 30*time.Second {
		t.Errorf("DefaultTTL = %v, want 30s", DefaultTTL)
	}

	if DefaultRetryInterval != 50*time.Millisecond {
		t.Errorf("DefaultRetryInterval = %v, want 50ms", DefaultRetryInterval)
	}
}
