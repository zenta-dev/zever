package main

import (
	"strings"
	"testing"

	"github.com/zenta-dev/zever/config"
)

// TestEnvTypoHintNamesTheOffendingKey pins the env guidance every
// env-resolving command shares: the hint names the exact key that carries
// the unknown field, and the key the developer probably meant.
func TestEnvTypoHintNamesTheOffendingKey(t *testing.T) {
	t.Parallel()

	env := config.Env{"DB_ADAPTERR": "sqlite"}
	err := &config.UnknownFieldError{Service: "db", Field: "ADAPTERR"}

	hint := envTypoHint(env, err)
	if !strings.Contains(hint, `"DB_ADAPTERR"`) {
		t.Fatalf("hint %q does not name the offending key", hint)
	}

	if !strings.Contains(hint, `"DB_ADAPTER"`) {
		t.Fatalf("hint %q does not suggest the adapter selector", hint)
	}
}

// TestEnvTypoHintNamesServiceKeysForAStrayVariable pins the ambient-variable
// case: a variable that matches no option (DB_HOST from a shell) reads as
// "unset it, or pick one of these", not as a dead end.
func TestEnvTypoHintNamesServiceKeysForAStrayVariable(t *testing.T) {
	t.Parallel()

	env := config.Env{"DB_HOST": "localhost"}
	err := &config.UnknownFieldError{Service: "db", Field: "HOST"}

	hint := envTypoHint(env, err)
	if !strings.Contains(hint, `"DB_HOST"`) {
		t.Fatalf("hint %q does not name the offending key", hint)
	}

	if !strings.Contains(hint, "unset it") {
		t.Fatalf("hint %q does not tell the developer to unset the stray variable", hint)
	}

	if !strings.Contains(hint, "DB_DSN") {
		t.Fatalf("hint %q does not list the service's real keys", hint)
	}
}

// TestEnvTypoHintEmptyForOtherErrors pins the contract: only an unknown
// field earns a hint, so other failures keep the plain error.
func TestEnvTypoHintEmptyForOtherErrors(t *testing.T) {
	t.Parallel()

	if hint := envTypoHint(config.Env{"DB_HOST": "localhost"}, notAFieldError{}); hint != "" {
		t.Fatalf("envTypoHint = %q, want \"\" for a non-unknown-field error", hint)
	}
}

// notAFieldError is a plain error that is not an unknown-field failure.
type notAFieldError struct{}

func (notAFieldError) Error() string { return "boom" }

// TestSplitEnvInstance pins the named-instance split: a default instance's
// remainder has no instance part, a named one does.
func TestSplitEnvInstance(t *testing.T) {
	t.Parallel()

	tests := []struct {
		rest     string
		wantInst string
		wantName string
	}{
		{rest: "HOST", wantInst: "", wantName: "HOST"},
		{rest: "_REPORTING_HOST", wantInst: "REPORTING", wantName: "HOST"},
		{rest: "_REPORTING", wantInst: "", wantName: "_REPORTING"},
	}

	for _, tc := range tests {
		inst, name := splitEnvInstance(tc.rest)
		if inst != tc.wantInst || name != tc.wantName {
			t.Fatalf("splitEnvInstance(%q) = (%q, %q), want (%q, %q)", tc.rest, inst, name, tc.wantInst, tc.wantName)
		}
	}
}

// TestServiceEnvKeysMatchTheEnvExample pins the key source: the keys a hint
// names are the same keys the .env.example generator writes.
func TestServiceEnvKeysMatchTheEnvExample(t *testing.T) {
	t.Parallel()

	keys := serviceEnvKeys("DB", "db")
	want := []string{"DB_ADAPTER", "DB_DSN", "DB_MAXCONNS"}

	for _, w := range want {
		found := false

		for _, k := range keys {
			if k == w {
				found = true

				break
			}
		}

		if !found {
			t.Fatalf("serviceEnvKeys(db) = %v, want it to contain %q", keys, w)
		}
	}
}

// TestWrapEnvConfigErrorNamesTheOffendingKey pins the shared wrapper every
// env-resolving command uses: the failure first, then the hint that names
// the key to fix.
func TestWrapEnvConfigErrorNamesTheOffendingKey(t *testing.T) {
	t.Setenv("ZEVER_NO_HINT", "")

	env := config.Env{"DB_HOST": "localhost"}
	err := &config.UnknownFieldError{Service: "db", Field: "HOST"}

	wrapped := wrapEnvConfigError("zever config show: resolve config from env", env, err)
	if wrapped == nil {
		t.Fatal("wrapEnvConfigError: got nil error")
	}

	msg := wrapped.Error()
	if !strings.HasPrefix(msg, "zever config show: resolve config from env: ") {
		t.Fatalf("error = %q, want the command prefix first", msg)
	}

	if !strings.Contains(msg, `"DB_HOST"`) {
		t.Fatalf("error = %q, want the hint to name the offending key", msg)
	}

	if !strings.Contains(msg, "unset it") {
		t.Fatalf("error = %q, want the hint to say what to do", msg)
	}
}

// TestWrapEnvConfigErrorSilencesHints pins ZEVER_NO_HINT: the env hint is a
// hint like every other, so silencing hints silences it and leaves the
// failure itself untouched.
func TestWrapEnvConfigErrorSilencesHints(t *testing.T) {
	t.Setenv("ZEVER_NO_HINT", "1")

	env := config.Env{"DB_HOST": "localhost"}
	err := &config.UnknownFieldError{Service: "db", Field: "HOST"}

	msg := wrapEnvConfigError("zever config show: resolve config from env", env, err).Error()
	if strings.Contains(msg, "hint:") {
		t.Fatalf("error carries a hint while ZEVER_NO_HINT is set:\n%s", msg)
	}

	if !strings.Contains(msg, `unknown field "HOST"`) {
		t.Fatalf("error = %q, want the original failure", msg)
	}
}

// TestWrapEnvConfigErrorWithoutAHint pins the plain path: a failure that
// names no unknown field keeps the wrapped error alone.
func TestWrapEnvConfigErrorWithoutAHint(t *testing.T) {
	t.Setenv("ZEVER_NO_HINT", "")

	msg := wrapEnvConfigError("zever config show: resolve config from env",
		config.Env{}, notAFieldError{}).Error()
	if strings.Contains(msg, "hint:") {
		t.Fatalf("error carries a hint for a failure that names no key:\n%s", msg)
	}

	if !strings.Contains(msg, "boom") {
		t.Fatalf("error = %q, want the wrapped failure", msg)
	}
}
