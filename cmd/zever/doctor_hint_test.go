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
			want:    "set AI_APIKEY in .env, then re-run 'zever doctor'",
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
			want:    "set GEO_USERAGENT in .env, then re-run 'zever doctor'",
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
			name:    "config row names the env key",
			battery: "config",
			errText: "config: unknown field \"AGENT\" for service \"ai\"",
			want:    "fix the SERVICE_FIELD env key named above, then re-run 'zever doctor'",
		},
		{
			name:    "adapters row names the adapter selector",
			battery: "adapters",
			errText: "config: unknown adapter \"memmory\" for service \"cache\"",
			want:    "set <SERVICE>_ADAPTER to a registered adapter ('zever explain' lists every driver)",
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
	// AI_APIKEY), so the hint must not claim AI_SECRET.
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
