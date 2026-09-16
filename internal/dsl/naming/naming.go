// Package naming provides string transformation utilities for DSL code generation.
package naming

import (
	"strings"
	"unicode"
)

// PascalCase converts a string to PascalCase.
// It handles snake_case by splitting on underscores, capitalizing each word, and joining them.
// Already-PascalCase strings are returned unchanged.
func PascalCase(s string) string {
	if s == "" {
		return ""
	}

	// If input contains underscores, it's snake_case
	if strings.Contains(s, "_") {
		parts := strings.Split(s, "_")
		for i, part := range parts {
			if part != "" {
				parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
			}
		}

		return strings.Join(parts, "")
	}

	// If no underscores, assume it's either already PascalCase or camelCase
	// Check if first letter is uppercase (PascalCase) or lowercase (camelCase)
	if len(s) > 0 && unicode.IsLower(rune(s[0])) {
		// camelCase -> convert to PascalCase
		return strings.ToUpper(s[:1]) + s[1:]
	}

	// Already PascalCase
	return s
}

// ScreamingSnake converts a string to SCREAMING_SNAKE_CASE.
// It handles camelCase, PascalCase, and snake_case input.
func ScreamingSnake(s string) string {
	if s == "" {
		return ""
	}

	// First convert to snake_case if needed
	snakeCase := toSnakeCase(s)

	// Convert to uppercase
	return strings.ToUpper(snakeCase)
}

// toSnakeCase is a helper that converts camelCase/PascalCase to snake_case.
func toSnakeCase(s string) string {
	if s == "" {
		return ""
	}

	// If it already contains underscores, it's already in snake_case
	if strings.Contains(s, "_") {
		return strings.ToLower(s)
	}

	// Convert camelCase/PascalCase to snake_case
	var result strings.Builder

	for i, r := range s {
		if i > 0 && unicode.IsUpper(r) {
			result.WriteRune('_')
		}

		result.WriteRune(unicode.ToLower(r))
	}

	return result.String()
}

// SnakeCase converts a string to snake_case. It handles camelCase,
// PascalCase, and already-snake_case input (returned lowercased
// unchanged), exposing the same conversion toSnakeCase already applies
// internally for ScreamingSnake, for callers (e.g. the zenorm
// backend) that need a lowercase snake_case identifier directly, such as
// an output file path derived from an entity name.
func SnakeCase(s string) string {
	return toSnakeCase(s)
}

// PluralizeNaive returns a naive English plural: a couple of common
// special cases (words ending in "y" -> "ies"; words ending in
// "s"/"x"/"ch"/"sh" -> "+es"), else a bare "+s" suffix. This is a known,
// documented simplification — it is wrong for irregular plurals — and is
// intentionally not used by the proto backend (Task 10; proto messages
// stay singular, matching entity names 1:1); it exists here as a shared,
// tested stub for the future Atlas/SQL backend's table naming, which does
// need pluralized names, so that limitation is decided once, in one place,
// rather than reinvented per backend. Replace with a real pluralizer
// library before relying on it for anything but English nouns.
func PluralizeNaive(s string) string {
	if s == "" {
		return ""
	}

	// Check for "y" ending
	if strings.HasSuffix(s, "y") {
		return s[:len(s)-1] + "ies"
	}

	// Check for "s", "x", "ch", "sh" endings
	if strings.HasSuffix(s, "s") || strings.HasSuffix(s, "x") ||
		strings.HasSuffix(s, "ch") || strings.HasSuffix(s, "sh") {
		return s + "es"
	}

	// Default: append "s"
	return s + "s"
}
