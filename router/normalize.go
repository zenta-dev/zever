package router

import (
	"regexp"
	"strings"
)

// braceParamRe matches the head of a brace param (`{name:`).
//
// It is the only regular expression executed by these helpers, and it is
// precompiled once at init. All other transforms below are pure byte scans
// over the pattern string: untrusted route patterns are never compiled or
// executed as regexps here, so route regex handling cannot introduce ReDoS.
// Go's regexp package itself is RE2 with linear-time matching guarantees.
var braceParamRe = regexp.MustCompile(`\{([a-zA-Z0-9_]+):`)

// ConvertColonSegments converts colon params in a single segment to
// brace form (`:name` to `{name}`, `:name<re>` to `{name:re}`).
//
// A bare `:` with no identifier passes through unchanged. A `:name<...>`
// without a closing `>` returns an error; callers log and skip the route.
func ConvertColonSegments(seg string) (string, error) {
	var b strings.Builder

	b.Grow(len(seg) + 8)

	for i := 0; i < len(seg); {
		if seg[i] != ':' {
			b.WriteByte(seg[i])
			i++

			continue
		}

		j := i + 1
		for j < len(seg) && isIdentChar(seg[j]) {
			j++
		}

		if j == i+1 {
			b.WriteByte(':')

			i++

			continue
		}

		name := seg[i+1 : j]

		if j < len(seg) && seg[j] == '<' {
			idx := strings.IndexByte(seg[j+1:], '>')
			if idx == -1 || idx == 0 {
				return "", &MalformedPatternError{Pattern: seg, Reason: "malformed colon regex: missing closing '>'"}
			}

			regex := seg[j+1 : j+1+idx]
			b.WriteString("{" + name + ":" + regex + "}")

			i = j + 1 + idx + 1

			continue
		}

		b.WriteString("{" + name + "}")

		i = j
	}

	return b.String(), nil
}

// isIdentChar reports whether c may appear in a colon-param name.
func isIdentChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

// BraceRegion returns the span of the brace param opened at loc.
//
// loc is a match from braceParamRe.FindAllStringSubmatchIndex; open is the
// index of `{`, end the index of the matching `}`, found with a depth
// counter so nested quantifiers like `[0-9]{4}` stay inside. ok is false
// when the brace never closes.
func BraceRegion(pattern string, loc []int) (open, end int, ok bool) {
	head := loc[3] + 1
	depth := 1

	for i := head; i < len(pattern); i++ {
		switch pattern[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return loc[0], i, true
			}
		}
	}

	return 0, 0, false
}

// ConvertColonRegex converts colon-style params to brace-style while
// leaving existing brace regions untouched.
//
// Unclosed trailing brace regions stop conversion and pass the remainder
// through unchanged. Malformed colon regexes return an error.
func ConvertColonRegex(pattern string) (string, error) {
	var b strings.Builder

	b.Grow(len(pattern) + 8)

	last := 0

	for _, loc := range braceParamRe.FindAllStringSubmatchIndex(pattern, -1) {
		open, end, ok := BraceRegion(pattern, loc)
		if !ok {
			break
		}

		seg, err := ConvertColonSegments(pattern[last:open])
		if err != nil {
			return "", err
		}

		b.WriteString(seg)
		b.WriteString(pattern[open : end+1])
		last = end + 1
	}

	tail, err := ConvertColonSegments(pattern[last:])
	if err != nil {
		return "", err
	}

	b.WriteString(tail)

	return b.String(), nil
}

// StripBraceRegexWrapper removes outer `regex(...)` wrappers inside brace
// params (`{id:regex(^...$)}` to `{id:^...$}`).
//
// An unclosed or otherwise invalid wrapper returns an error; callers log
// and skip the route instead of registering a half-parsed pattern.
func StripBraceRegexWrapper(pattern string) (string, error) {
	var b strings.Builder

	b.Grow(len(pattern))

	last := 0

	for _, loc := range braceParamRe.FindAllStringSubmatchIndex(pattern, -1) {
		open, end, ok := BraceRegion(pattern, loc)
		if !ok {
			break
		}

		innerRe := pattern[loc[3]+1 : end]
		// Validate wrapper is properly closed if it starts with regex(
		if strings.HasPrefix(innerRe, "regex(") {
			stripped := StripRegexWrapper(innerRe)
			// If still starts with regex( after strip, it was malformed/unclosed
			// StripRegexWrapper returns inner without prefix for malformed, so detect
			// original malformed by checking anchored validity
			if stripped == innerRe {
				// No stripping happened, but prefix exists — check if unclosed
				// Re-run validation: if depth never 0 or not anchored, it's malformed
				// For strictness, return error
				if !isValidRegexWrapper(innerRe) {
					return "", &MalformedPatternError{Pattern: pattern, Reason: "malformed regex wrapper"}
				}
			}

			innerRe = stripped
		}

		b.WriteString(pattern[last:open])
		b.WriteString("{" + pattern[open+1:loc[3]] + ":" + innerRe + "}")

		last = end + 1
	}

	b.WriteString(pattern[last:])

	return b.String(), nil
}

// isValidRegexWrapper reports whether re is a well-formed anchored regex(...) wrapper.
func isValidRegexWrapper(re string) bool {
	inner, ok := strings.CutPrefix(re, "regex(")
	if !ok {
		return true
	}

	depth := 1

	for i := 0; i < len(inner); i++ {
		switch inner[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i == len(inner)-1
			}
		}
	}

	return false
}

// ChiPattern converts colon syntax to brace form and strips regex wrappers.
//
// It is the chi driver's normalization entry point: `:name` becomes
// `{name}`, `:name<re>` becomes `{name:re}`, and `{name:regex(re)}`
// becomes `{name:re}`. Errors describe malformed input for log-and-skip.
func ChiPattern(pattern string) (string, error) {
	converted, err := ConvertColonRegex(pattern)
	if err != nil {
		return "", err
	}

	return StripBraceRegexWrapper(converted)
}
