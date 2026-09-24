package header

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zenta-dev/zever/tenant"
)

func TestResolveReDoSSafe(t *testing.T) {
	t.Parallel()

	// Valid-but-slow-ish pattern that passes the guard: single quantified
	// group anchored with a trailing literal. Must resolve a 1KB host fast.
	pattern := `^([a-z]+)b$`
	tt, err := New(tenant.Options{Header: "X-Tenant-ID", SubdomainRegex: pattern})
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	host := strings.Repeat("a", 1024) + "b"
	meta := map[string]string{"Host": host}

	start := time.Now()
	_, resolveErr := tt.Resolve(t.Context(), meta)
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Fatalf("Resolve took too long (%v) with 1KB host - ReDoS vulnerable", elapsed)
	}

	// 1KB exceeds the 253-char DNS cap, so the host limit (not the
	// regex) must reject it — quickly. Either outcome proves bounded time.
	if !errors.Is(resolveErr, ErrHostTooLong) {
		t.Fatalf("want ErrHostTooLong, got %v", resolveErr)
	}
}

func TestOpenLongPatternRejected(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("a", tenant.MaxRegexLength+1)

	if !isCatastrophicPattern(long) {
		t.Fatalf("isCatastrophicPattern(%d chars) = false, want true", len(long))
	}

	_, err := New(tenant.Options{Header: "X-Tenant-ID", SubdomainRegex: long})
	if err == nil {
		t.Fatal("want error for 501-char pattern, got nil")
	}

	// Open validates core Options first, so an over-long pattern surfaces
	// as InvalidOptions before the catastrophic-pattern guard runs.
	if !errors.Is(err, tenant.ErrInvalidOptions) {
		t.Fatalf("want tenant.ErrInvalidOptions, got %v", err)
	}
}

func TestIsCatastrophicPatternNested(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		pattern string
		want    bool
	}{
		{"star nesting", "(a*)*", true},
		{"plus nesting", "^(a+)+$", true},
		{"quest nesting", "(a?)?", true},
		{"repeat nesting", "(a{2,3})+", true},
		{"flat star", "^a*$", false},
		{"flat plus", "^a+$", false},
		{"flat quest", "^a?$", false},
		{"flat repeat", "^a{2,3}$", false},
		{"flat group", "^([a-z]+)\\.example\\.com$", false},
		{"dot star plus", "(.*)+", true},
		{"plus plus", "a++", true},
		{"star plus", "a*+b", true},
		{"repeat-plus nesting no paren-adjacent", "^(a+)?$", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := isCatastrophicPattern(tc.pattern); got != tc.want {
				t.Errorf("isCatastrophicPattern(%q) = %v, want %v", tc.pattern, got, tc.want)
			}
		})
	}
}

func TestIsCatastrophicPatternParseFailure(t *testing.T) {
	t.Parallel()

	if isCatastrophicPattern("(unclosed") {
		t.Fatal("isCatastrophicPattern(\"(unclosed\") = true, want false")
	}
}

func TestIsCatastrophicPatternValid(t *testing.T) {
	t.Parallel()

	for _, pat := range []string{
		`^([a-z0-9-]+)\.example\.com$`,
		`^([a-z]+)b$`,
		`^tenant-[0-9]+$`,
	} {
		if isCatastrophicPattern(pat) {
			t.Errorf("isCatastrophicPattern(%q) = true, want false", pat)
		}
	}
}

func TestResolveHostLengthLimit(t *testing.T) {
	t.Parallel()

	tt, err := New(tenant.Options{
		Header:         "X-Tenant-ID",
		SubdomainRegex: `^([a-z0-9-]+)\.example\.com$`,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	host := strings.Repeat("a", 300) + ".example.com"
	meta := map[string]string{"Host": host}

	start := time.Now()
	_, resolveErr := tt.Resolve(t.Context(), meta)
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Fatalf("Resolve with long host took too long: %v", elapsed)
	}

	if !errors.Is(resolveErr, ErrHostTooLong) {
		t.Fatalf("want ErrHostTooLong, got %v", resolveErr)
	}
}

func TestOpenCatastrophicPatternsRejected(t *testing.T) {
	t.Parallel()

	cases := []string{
		"^(a+)+$",
		"(a+)+",
		"(.*)+",
		"a++",
		"a*+b",
	}

	for _, pat := range cases {
		t.Run(pat, func(t *testing.T) {
			t.Parallel()

			_, err := New(tenant.Options{Header: "X-Tenant-ID", SubdomainRegex: pat})
			if err == nil {
				t.Errorf("want error for catastrophic pattern %q, got nil", pat)
			} else if !errors.Is(err, ErrInvalidPattern) {
				t.Errorf("want ErrInvalidPattern for %q, got %v", pat, err)
			}
		})
	}
}
