package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestDoctorFixHintNamesMissingEnvKey pins the doctor fix hint: a failing
// battery whose error names a missing option gets the exact env key to set,
// derived from the battery's own Options struct (the same source the
// .env.example generator uses).
func TestDoctorFixHintNamesMissingEnvKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		battery string
		errText string
		want    string
	}{
		{
			name:    "ai api key",
			battery: "ai",
			errText: "container: ai: ai: open anthropic: ai: open anthropic: ai: invalid options: api_key is required",
			want:    "set AI_API_KEY in .env, then re-run 'zever doctor'",
		},
		{
			name:    "auth jwt secret",
			battery: "auth",
			errText: "container: auth: auth: open jwt: auth: invalid options: jwt secret is required",
			want:    "set AUTH_JWT_SECRET in .env, then re-run 'zever doctor'",
		},
		{
			name:    "db quoted option",
			battery: "db",
			errText: "db: postgres: option \"dsn\" is required",
			want:    "set DB_DSN in .env, then re-run 'zever doctor'",
		},
		{
			name:    "geo user agent",
			battery: "geo",
			errText: "geo: open osm: osm: user_agent is required",
			want:    "set GEO_USER_AGENT in .env, then re-run 'zever doctor'",
		},
		{
			name:    "crypto key",
			battery: "crypto",
			errText: "crypto: invalid options: key is required",
			want:    "set CRYPTO_KEY in .env, then re-run 'zever doctor'",
		},
		{
			name:    "unknown field falls back",
			battery: "i18n",
			errText: "i18n: invalid options: embed fs is required",
			want:    "run 'zever config show' to see the resolved i18n values",
		},
		{
			name:    "no missing field falls back",
			battery: "geo",
			errText: "geo: open static: static: read cities: open cities.json: no such file or directory",
			want:    "run 'zever config show' to see the resolved geo values",
		},
		{
			name:    "config row names the service",
			battery: "config",
			errText: "config: invalid options auth: jwt secret must be at least 32 bytes",
			want:    "fix the service env the error names, then re-run 'zever doctor'",
		},
		{
			name:    "unknown battery falls back to config show",
			battery: "adapters",
			errText: "config: unknown adapter \"memmory\" for service \"cache\"",
			want:    "run 'zever config show' to see the resolved adapters values",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := doctorFixHint(tc.battery, tc.errText); got != tc.want {
				t.Fatalf("doctorFixHint(%q, %q) = %q, want %q", tc.battery, tc.errText, got, tc.want)
			}
		})
	}
}

// TestDoctorFixHintNeverInventsAKey pins the safety rule: a hint names an
// env key only when the battery really declares that option, so the
// developer never gets a key that does not exist.
func TestDoctorFixHintNeverInventsAKey(t *testing.T) {
	t.Parallel()

	// "secret" alone is not a field of the ai battery (its key is
	// AI_API_KEY), so the hint must not claim AI_SECRET.
	if got := doctorFixHint("ai", "ai: invalid options: secret is required"); strings.Contains(got, "AI_SECRET") {
		t.Fatalf("doctorFixHint invented a key: %q", got)
	}
}

// TestDoctorHumanOutputNamesFixPerFailingRow pins the human table: every
// failing row carries its own next action, so a developer never has to
// guess which env key a failing battery wants.
func TestDoctorHumanOutputNamesFixPerFailingRow(t *testing.T) {
	dir := t.TempDir()
	writeInspectFixture(t, dir, ".env", doctorFailingBatteryFixture)
	t.Chdir(dir)

	t.Setenv("ZEVER_NO_HINT", "")

	var out bytes.Buffer
	if err := runDoctorWith(DoctorConfig{Out: &out}); err == nil {
		t.Fatal("runDoctorWith failing battery: got nil error, want non-nil")
	}

	got := out.String()
	if !strings.Contains(got, "hint: set AUTH_JWT_SECRET in .env, then re-run 'zever doctor'") {
		t.Fatalf("output lacks the auth fix hint:\n%s", got)
	}
}

// TestDoctorHumanOutputHidesFixWhenHintsSuppressed pins ZEVER_NO_HINT: the
// per-row fix is a hint like every other, so silencing hints silences it.
func TestDoctorHumanOutputHidesFixWhenHintsSuppressed(t *testing.T) {
	dir := t.TempDir()
	writeInspectFixture(t, dir, ".env", doctorFailingBatteryFixture)
	t.Chdir(dir)

	t.Setenv("ZEVER_NO_HINT", "1")

	var out bytes.Buffer
	if err := runDoctorWith(DoctorConfig{Out: &out}); err == nil {
		t.Fatal("runDoctorWith failing battery: got nil error, want non-nil")
	}

	if got := out.String(); strings.Contains(got, "hint: set AUTH_JWT_SECRET") {
		t.Fatalf("output carries a hint while ZEVER_NO_HINT is set:\n%s", got)
	}
}

// TestDoctorFooterPointsAtThePerRowFix pins the footer: it tells the
// developer where the next action is, instead of naming an env key that
// not every failing row has.
func TestDoctorFooterPointsAtThePerRowFix(t *testing.T) {
	dir := t.TempDir()
	writeInspectFixture(t, dir, ".env", doctorFailingBatteryFixture)
	t.Chdir(dir)

	t.Setenv("ZEVER_NO_HINT", "")

	var out bytes.Buffer

	var err error

	stderr := captureZeverStderr(t, func() {
		err = runDoctorWith(DoctorConfig{Out: &out})
	})
	if err == nil {
		t.Fatal("runDoctorWith failing battery: got nil error, want non-nil")
	}

	if !strings.Contains(stderr, "each FAIL row above names the fix") {
		t.Fatalf("footer does not point at the per-row fix:\n%s", stderr)
	}
}

// TestDoctorEnvFailureNamesTheOffendingKey pins the doctor path that fails
// before any battery runs: an unknown env key under a wired service prefix
// names the key to fix.
func TestDoctorEnvFailureNamesTheOffendingKey(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("ZEVER_NO_HINT", "")
	t.Setenv("DB_MAXCONN", "5")

	err := runDoctorWith(DoctorConfig{Out: &bytes.Buffer{}})
	if err == nil {
		t.Fatal("runDoctorWith typo env key: got nil error, want non-nil")
	}

	if !strings.Contains(err.Error(), `"DB_MAXCONN"`) {
		t.Fatalf("error = %q, want the hint to name the offending key", err.Error())
	}

	if !strings.Contains(err.Error(), `"DB_MAX_CONNS"`) {
		t.Fatalf("error = %q, want the hint to name the corrected key", err.Error())
	}
}
