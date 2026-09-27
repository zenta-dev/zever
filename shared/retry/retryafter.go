package retry

import (
	"net/http"
	"strconv"
	"time"
)

// ParseRetryAfter parses an HTTP Retry-After header value relative to now,
// supporting both forms RFC 9110 allows: delay-seconds (an integer or
// floating-point number of seconds, e.g. "120") and an HTTP-date (e.g. "Wed,
// 21 Oct 2026 07:28:00 GMT"). It returns (0, false) if header is empty or
// unparseable in either form, and (0, true) if a valid HTTP-date has already
// passed (a zero-or-negative wait, not an error).
func ParseRetryAfter(header string, now time.Time) (time.Duration, bool) {
	if header == "" {
		return 0, false
	}

	if secs, err := strconv.ParseFloat(header, 64); err == nil {
		if secs < 0 {
			secs = 0
		}

		return time.Duration(secs * float64(time.Second)), true
	}

	if t, err := http.ParseTime(header); err == nil {
		d := t.Sub(now)
		if d < 0 {
			d = 0
		}

		return d, true
	}

	return 0, false
}

// ParseRetryAfterMs parses an Anthropic-style Retry-After-Ms header value: a
// delay in milliseconds, as an integer or floating-point number (no
// HTTP-date form exists for this header). It returns (0, false) if header is
// empty or unparseable.
func ParseRetryAfterMs(header string) (time.Duration, bool) {
	if header == "" {
		return 0, false
	}

	ms, err := strconv.ParseFloat(header, 64)
	if err != nil {
		return 0, false
	}

	if ms < 0 {
		ms = 0
	}

	return time.Duration(ms * float64(time.Millisecond)), true
}
