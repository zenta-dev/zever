package http

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// FuzzSignEnvelope proves the delivery-side signature envelope never panics
// on arbitrary secrets, payloads, and timestamps, and always round-trips:
// the envelope parses as "t=<ts>,v1=<hex>", the v1 MAC independently
// recomputes as HMAC-SHA256 over "<ts>.<payload>", and signing is
// deterministic. A silent algorithm or format change breaks this target.
func FuzzSignEnvelope(f *testing.F) {
	seeds := []struct {
		secret  string
		payload []byte
		ts      int64
	}{
		{"secret", []byte("{}"), 1_700_000_000},
		{"", []byte("{}"), 1_700_000_000},
		{"secret", nil, 1_700_000_000},
		{"secret", []byte{}, 0},
		{"secret", []byte("{}"), -1},
		{"secret", []byte("\x00\xff\xfe"), 1_700_000_000},
		{"s", []byte(`{"id":"evt_1"}`), 9_999_999_999},
		{"\x00", []byte("\x00"), 1_700_000_000},
	}

	for _, seed := range seeds {
		f.Add(seed.secret, seed.payload, seed.ts)
	}

	f.Fuzz(func(t *testing.T, secret string, payload []byte, ts int64) {
		t.Parallel()

		got := signAt(secret, payload, ts)
		if got == "" {
			t.Fatal("signAt() returned empty envelope")
		}

		// The envelope must carry the exact timestamp it was signed with.
		wantPrefix := "t=" + strconv.FormatInt(ts, 10) + ",v1="
		macHex, ok := strings.CutPrefix(got, wantPrefix)
		if !ok {
			t.Fatalf("signAt() = %q, want prefix %q", got, wantPrefix)
		}

		if len(macHex) != 64 {
			t.Fatalf("signAt() v1 = %q, want 64 hex chars", macHex)
		}

		// The MAC must independently recompute: guards against a silent
		// change of algorithm, key, or signed message layout.
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(strconv.FormatInt(ts, 10)))
		mac.Write([]byte{'.'})
		mac.Write(payload)

		if want := hex.EncodeToString(mac.Sum(nil)); macHex != want {
			t.Fatalf("signAt() v1 = %q, independently recomputed %q", macHex, want)
		}

		// Signing is deterministic.
		if again := signAt(secret, payload, ts); again != got {
			t.Fatalf("signAt() not deterministic: %q vs %q", got, again)
		}
	})
}

// FuzzRedactTarget proves target redaction never panics and always masks:
// when the input parses as a URL carrying userinfo with a password, the
// redacted output must re-parse with exactly the mask as its password.
func FuzzRedactTarget(f *testing.F) {
	seeds := []string{
		"",
		"https://example.com/hook",
		"https://user:pass@example.com/hook",
		"https://user@example.com/hook",
		"https://:pass@example.com/hook",
		"http://user:p@ss:w0rd@example.com:8080/a?b=c",
		"not a url",
		"://missing-scheme",
		"https://user:xxxxx@example.com/hook",
		"p://:x@", // single-char password that substrings the mask
		"\x00\xff\xfe",
		"https://user:pass@example.com",
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, target string) {
		t.Parallel()

		got := redactTarget(target)

		u, err := url.Parse(target)
		if err != nil || u.User == nil {
			return
		}

		pw, hasPassword := u.User.Password()
		if !hasPassword || pw == "" {
			return
		}

		// The redacted output must re-parse with exactly the mask as its
		// password: replacement, not echo. Comparing parsed passwords
		// instead of substrings avoids false-positives when the real
		// password is itself a substring of the mask (e.g. "x").
		redacted, err := url.Parse(got)
		if err != nil {
			t.Fatalf("redactTarget(%q) = %q, does not re-parse: %v", target, got, err)
		}

		redactedPw, hasRedactedPassword := redacted.User.Password()
		if !hasRedactedPassword {
			t.Fatalf("redactTarget(%q) = %q, dropped userinfo", target, got)
		}

		if redactedPw != "xxxxx" {
			t.Fatalf("redactTarget(%q) = %q, password not masked", target, got)
		}
	})
}
