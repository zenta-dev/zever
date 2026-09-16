// Package router provides router functionality.
package router

import (
	"context"
	"net/http"
	"strings"
)

type paramsKey struct{}

// WithParams stores route params in the request context.
//
// It is nil-safe and copies the map to avoid aliasing the caller's map
// and leaking pooled instances. Mutating params after the call does not
// affect the stored copy.
func WithParams(ctx context.Context, params map[string]string) context.Context {
	if params == nil {
		return context.WithValue(ctx, paramsKey{}, params)
	}

	cp := make(map[string]string, len(params))
	for k, v := range params {
		cp[k] = v
	}

	return context.WithValue(ctx, paramsKey{}, cp)
}

// Param returns the named route parameter from the request.
//
// It returns "" when the request carries no params or the name is absent.
func Param(r *http.Request, name string) string {
	if params, ok := r.Context().Value(paramsKey{}).(map[string]string); ok {
		return params[name]
	}

	return ""
}

// ParamNames returns all route parameters from the request as a copy.
//
// It returns nil when the request carries no params. Mutating the result
// does not affect the stored params.
func ParamNames(r *http.Request) map[string]string {
	if params, ok := r.Context().Value(paramsKey{}).(map[string]string); ok {
		out := make(map[string]string, len(params))
		for k, v := range params {
			out[k] = v
		}

		return out
	}

	return nil
}

// NormalizePattern converts brace-style params to colon-style segments.
//
// `{name}` becomes `:name` and `{name:re}` becomes
// `:name<regex(^(?:re)$)>`. Nested braces are consumed as part of the
// outer body, unbalanced input passes through unchanged, and malformed
// wrappers are never double-wrapped (see StripRegexWrapper).
func NormalizePattern(pattern string) string {
	var b strings.Builder

	b.Grow(len(pattern) + 8)

	depth := 0
	start := -1

	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '{':
			if depth == 0 {
				start = i
			}

			depth++
		case '}':
			if depth == 0 {
				b.WriteByte('}')
				continue
			}

			depth--
			if depth > 0 {
				continue
			}

			body := pattern[start+1 : i]
			name, re, _ := strings.Cut(body, ":")

			switch {
			case name == "":
				b.WriteString(pattern[start : i+1])
			case re == "":
				b.WriteByte(':')
				b.WriteString(name)
			default:
				re = StripRegexWrapper(re)
				b.WriteString(":" + name + "<regex(^(?:" + re + ")$)>")
			}

			start = -1
		default:
			if depth == 0 {
				b.WriteByte(pattern[i])
			}
		}
	}

	if start >= 0 {
		b.WriteString(pattern[start:])
	}

	return b.String()
}

// StripRegexWrapper removes one outer `regex(...)` wrapper from re.
//
// It is anchored: the wrapper only strips when its closing paren is the
// last byte of the string, otherwise re is returned unchanged. A malformed
// (unclosed) wrapper returns the inner content without the `regex(` prefix
// so callers do not double-wrap it.
func StripRegexWrapper(re string) string {
	inner, wrapped := strings.CutPrefix(re, "regex(")
	if !wrapped {
		return re
	}

	depth := 1

	for i := 0; i < len(inner); i++ {
		switch inner[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				if i != len(inner)-1 {
					return re
				}

				return inner[:i]
			}
		}
	}

	return inner
}
