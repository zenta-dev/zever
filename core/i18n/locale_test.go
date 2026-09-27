package i18n

import (
	"reflect"
	"testing"
)

func TestBaseLocale_table(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want string
	}{
		{"en-US", "en"},
		{"zh_Hant", "zh"},
		{"en", "en"},
		{"", ""},
		{"fr-FR", "fr"},
		{"pt_BR", "pt"},
		{"EN-US", "EN"},
		{"en-US-variant", "en"},
	}
	for _, c := range cases {
		if got := BaseLocale(c.in); got != c.want {
			t.Errorf("BaseLocale(%q) = %q want %q", c.in, got, c.want)
		}
	}
}

func TestLocaleChain_resolutionOrder(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		locale   string
		fallback string
		want     []string
	}{
		{"full with fallback", "en-US", "en", []string{"en-US", "en"}},
		{"bare with distinct fallback", "en", "fr", []string{"en", "fr"}},
		{"bare matching fallback deduped", "en", "en", []string{"en"}},
		{"empty fallback", "en-US", "", []string{"en-US", "en"}},
		{"bare no fallback", "en", "", []string{"en"}},
		{"fallback distinct from base", "en-US", "fr", []string{"en-US", "en", "fr"}},
		{"fallback equals locale deduped", "en-US", "en-US", []string{"en-US", "en"}},
		{"fallback equals base deduped", "zh_Hant", "zh", []string{"zh_Hant", "zh"}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := LocaleChain(c.locale, c.fallback); !reflect.DeepEqual(got, c.want) {
				t.Errorf("LocaleChain(%q, %q) = %q want %q", c.locale, c.fallback, got, c.want)
			}
		})
	}
}
