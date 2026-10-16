package middleware

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// maxPanicValueLen caps the logged panic value in bytes.
const maxPanicValueLen = 1024

const redacted = "[REDACTED]"

// panicRedactions scrub credentials a panic value may carry (errors often
// embed DSNs, headers or request fragments). Order matters: structured forms
// run before the generic long-token sweep.
var panicRedactions = []struct {
	re   *regexp.Regexp
	repl string
}{
	// scheme://user:password@host
	{regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://)[^\s/@:]+:[^\s/@]+@`), "${1}" + redacted + "@"},
	// Authorization schemes.
	{regexp.MustCompile(`(?i)\b(bearer|basic)\s+[A-Za-z0-9._~+/=-]+`), "${1} " + redacted},
	// key=value, key: value and "key":"value" for sensitive keys.
	{regexp.MustCompile(`(?i)("?(?:password|passwd|pwd|secret|token|api[_-]?key|apikey|authorization|credentials?|private[_-]?key|dsn)"?\s*[:=]\s*)("[^"]*"|'[^']*'|[^\s,;&}]+)`), "${1}" + redacted},
	// Long opaque tokens (session IDs, keys): 32+ hex or 40+ url-safe chars.
	{regexp.MustCompile(`\b[A-Fa-f0-9]{32,}\b`), redacted},
	{regexp.MustCompile(`\b[A-Za-z0-9_-]{40,}\b`), redacted},
}

// redactPanicValue renders a recovered panic value for logs with credentials
// scrubbed and the length capped. The client never sees this value; the
// scrubbing protects log sinks, which are far more widely readable than the
// process memory the secrets came from.
func redactPanicValue(p any) string {
	s := fmt.Sprint(p)
	for _, r := range panicRedactions {
		s = r.re.ReplaceAllString(s, r.repl)
	}

	if len(s) > maxPanicValueLen {
		cut := maxPanicValueLen
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}

		s = strings.ToValidUTF8(s[:cut], "") + "...(truncated)"
	}

	return s
}
