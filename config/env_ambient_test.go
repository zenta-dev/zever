package config

import (
	"errors"
	"reflect"
	"testing"
)

// ambientKeys are the unrelated environment variables that used to break
// resolution: every AI coding agent exports AI_AGENT, and LOG_LEVEL,
// DB_URL and CACHE_DIR are ambient in a large share of Go, Python and Node
// projects.
var ambientKeys = []string{
	"AI_AGENT",
	"LOG_LEVEL",
	"DB_URL",
	"CACHE_DIR",
	"AGENT",
	"FOO_BAR",
	"DB_NOPE_X",
	"EVENTBUS_ONPANIC",
	"SCHEDULER_DISPATCHER",
	"AUTH_TOKEN",
}

// ambientServiceKeys are the ambient keys that sit under a wired service
// prefix. Only these are affected by the strict switch: a key whose head
// names no service (AGENT, FOO_BAR) is ignored in every mode, exactly as
// the v0.8 rule already did.
var ambientServiceKeys = []string{
	"AI_AGENT",
	"LOG_LEVEL",
	"DB_URL",
	"CACHE_DIR",
	"DB_NOPE_X",
	"EVENTBUS_ONPANIC",
	"SCHEDULER_DISPATCHER",
	"AUTH_TOKEN",
}

// TestEnvIgnoresAmbientVariables pins the ambient-environment rule: a key
// under a wired service prefix that resolves to no field and is close to no
// known field is an unrelated ambient variable. It must not fail resolution
// and it must not land on any field.
func TestEnvIgnoresAmbientVariables(t *testing.T) {
	t.Parallel()
	for _, key := range ambientKeys {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			assertUnapplied(t, Env{key: "ambient-value"})
		})
	}
}

// TestEnvIgnoresAmbientVariablesTogether pins the rule for a whole noisy
// environment: many ambient variables at once still resolve, and none of
// them changes a single field.
func TestEnvIgnoresAmbientVariablesTogether(t *testing.T) {
	t.Parallel()
	env := Env{}
	for _, key := range ambientKeys {
		env[key] = "ambient-value"
	}
	assertUnapplied(t, env)
}

// TestEnvStrictModeRejectsAmbientKeys pins the opt-in strict mode:
// StrictEnvKey turns the ambient variables back into errors, restoring the
// v0.8 behavior of rejecting every unknown key under a wired prefix.
func TestEnvStrictModeRejectsAmbientKeys(t *testing.T) {
	t.Parallel()
	for _, key := range ambientServiceKeys {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			env := Env{key: "ambient-value", StrictEnvKey: "1"}
			_, err := Resolve(env)
			if err == nil {
				t.Fatalf("%s: want error in strict mode", key)
			}
			var unknown *UnknownFieldError
			if !errors.As(err, &unknown) {
				t.Fatalf("%s: want UnknownFieldError, got %T: %v", key, err, err)
			}
			if !errors.Is(err, ErrUnknownField) {
				t.Fatalf("%s: want ErrUnknownField match: %v", key, err)
			}
		})
	}
}

// TestEnvStrictModeValues pins which values of the strict switch turn the
// strict mode on: a true value enables it, a false or unparsable one leaves
// the default best-effort rule in place.
func TestEnvStrictModeValues(t *testing.T) {
	t.Parallel()
	tests := []struct {
		value  string
		strict bool
	}{
		{"1", true},
		{"true", true},
		{"TRUE", true},
		{" 1 ", true},
		{"0", false},
		{"false", false},
		{"", false},
		{"maybe", false},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			t.Parallel()
			env := Env{"DB_NOPE_X": "x", StrictEnvKey: tt.value}
			_, err := Resolve(env)
			if tt.strict && err == nil {
				t.Fatalf("strict = %q: want error", tt.value)
			}
			if !tt.strict && err != nil {
				t.Fatalf("strict = %q: want ignored, got %v", tt.value, err)
			}
		})
	}
}

// TestEnvStrictModeKeepsKnownFields pins that the strict switch changes
// nothing else: known fields still apply and the switch itself never lands
// on a field.
func TestEnvStrictModeKeepsKnownFields(t *testing.T) {
	t.Parallel()
	env := Env{"DB_MAXCONNS": "7", StrictEnvKey: "1"}
	cfg, err := Resolve(env)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DB.Options.MaxConns != 7 {
		t.Fatalf("got %d", cfg.DB.Options.MaxConns)
	}
	for name, svc := range cfg.RedactedServices() {
		for key, value := range svc.Options {
			if value == "1" {
				t.Fatalf("strict switch leaked into %s.%s", name, key)
			}
		}
	}
}

// TestUnresolvedKeys pins the helper that reports the keys Resolve ignores,
// so a caller (for example `zever doctor`) can surface them. A key with a
// suggestion is a near-miss typo.
func TestUnresolvedKeys(t *testing.T) {
	t.Parallel()
	env := Env{
		"DB_URL":          "postgres://x",
		"DB_MAXCONN":      "20",
		"DB_MAXCONNS":     "20",
		"AI_AGENT":        "opencode",
		"AUTH_JWT_SECRT":  "s",
		"PATH":            "/bin",
		"DB__REPORT__DSN": "x",
	}
	got := UnresolvedKeys(env)
	want := []UnresolvedKey{
		{Key: "AI_AGENT", Service: "ai", Field: "AGENT"},
		{Key: "AUTH_JWT_SECRT", Service: "auth", Field: "JWT_SECRT", Suggestion: "secret"},
		{Key: "DB_MAXCONN", Service: "db", Field: "MAXCONN", Suggestion: "max_conns"},
		{Key: "DB_URL", Service: "db", Field: "URL"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if out := UnresolvedKeys(nil); out != nil {
		t.Fatalf("nil env: got %+v", out)
	}
}

// assertUnapplied resolves env and fails when resolution errors or when the
// resolved config differs from the defaults: an ignored key must not land on
// any field.
func assertUnapplied(t *testing.T, env Env) {
	t.Helper()
	cfg, err := Resolve(env)
	if err != nil {
		t.Fatalf("Resolve(%v): %v", env, err)
	}
	want := Default().RedactedServices()
	got := cfg.RedactedServices()
	for name, svc := range got {
		if !reflect.DeepEqual(svc, want[name]) {
			t.Fatalf("service %q changed: got %+v, want %+v", name, svc, want[name])
		}
	}
}
