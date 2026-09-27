package i18n

import (
	"strings"
)

// BaseLocale returns the base language subtag of locale.
// It splits on '-' or '_' and returns the first subtag, preserving case:
// "en-US" becomes "en", "zh_Hant" becomes "zh", "en" stays "en".
// Surrounding whitespace is trimmed; empty input returns empty.
func BaseLocale(locale string) string {
	s := strings.TrimSpace(locale)
	if i := strings.IndexAny(s, "-_"); i >= 0 {
		return s[:i]
	}
	return s
}

// LocaleChain returns the lookup chain for locale with fallback.
// Resolution order is: the full locale first, then its base language
// (when different), then fallback (when non-empty and distinct).
// Entries are deduplicated, preserving first occurrence.
func LocaleChain(locale, fallback string) []string {
	out := make([]string, 0, 3)
	locale = strings.TrimSpace(locale)
	fallback = strings.TrimSpace(fallback)
	if locale != "" {
		out = append(out, locale)
	}
	if base := BaseLocale(locale); base != "" && base != locale {
		out = append(out, base)
	}
	if fallback != "" {
		dup := false
		for _, e := range out {
			if e == fallback {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, fallback)
		}
	}
	return out
}
