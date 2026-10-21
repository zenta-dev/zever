package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/config"
)

// TestEnvTypoHintPicksTheSameKeyEveryTime pins the determinism: the env is
// a map, so the hint must walk its keys in sorted order, or two runs with
// the same environment could name different keys.
func TestEnvTypoHintPicksTheSameKeyEveryTime(t *testing.T) {
	t.Parallel()

	env := config.Env{
		"DB_MAXCONN":            "5",
		"DB__REPORTING_MAXCONN": "5",
	}
	err := &config.UnknownFieldError{Service: "db", Field: "MAXCONN"}

	first := envTypoHint(env, err)
	if first == "" {
		t.Fatal("envTypoHint = \"\", want a hint")
	}

	for range 20 {
		if got := envTypoHint(env, err); got != first {
			t.Fatalf("envTypoHint = %q, want the stable %q", got, first)
		}
	}

	// The default instance's key wins over a named instance's.
	if !strings.Contains(first, `"DB_MAXCONN"`) {
		t.Fatalf("hint %q does not name the default instance's key", first)
	}
}

// TestEnvTypoHintStrictModeNamesTheRealKeys pins the strict-mode guidance
// end to end: with ZEVER_CONFIG_STRICT=1 an unrelated variable fails
// resolution again, and the hint says what to do with it instead of
// dead-ending.
//
// The environment is built explicitly instead of resolved from the process:
// strict mode rejects every unknown key under a wired service prefix, so a
// test that read the real environment would fail in any shell that exports
// an ambient key (AI_AGENT and friends) before the test's own typo is
// reached.
func TestEnvTypoHintStrictModeNamesTheRealKeys(t *testing.T) {
	t.Setenv("ZEVER_NO_HINT", "")

	env := config.Env{
		config.StrictEnvKey: "1",
		"DB_HOST":           "localhost",
	}

	err := runConfigShowWithEnv(ConfigConfig{Out: &bytes.Buffer{}}, env)
	if err == nil {
		t.Fatal("runConfigShowWithEnv strict ambient key: got nil error, want non-nil")
	}

	msg := err.Error()
	if !strings.Contains(msg, `"DB_HOST"`) {
		t.Fatalf("error = %q, want the hint to name the offending key", msg)
	}

	if !strings.Contains(msg, "unset it") {
		t.Fatalf("error = %q, want the hint to say what to do", msg)
	}

	if !strings.Contains(msg, "DB_DSN") {
		t.Fatalf("error = %q, want the hint to list the service's real keys", msg)
	}
}

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
	want := []string{"DB_ADAPTER", "DB_DSN", "DB_MAX_CONNS"}

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

// TestEnvKeyChoicesStayTerminalReadable pins the cap on the key list: the
// hint stays short enough to read in a terminal, and the ellipsis marks
// that the list continues.
func TestEnvKeyChoicesStayTerminalReadable(t *testing.T) {
	t.Parallel()

	choices := envKeyChoices("db", "")
	if len(choices) > maxEnvKeyChoices+1 {
		t.Fatalf("envKeyChoices(db) = %v, want at most %d entries", choices, maxEnvKeyChoices+1)
	}

	if choices[len(choices)-1] != "…" {
		t.Fatalf("envKeyChoices(db) = %v, want the list to end with an ellipsis", choices)
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
