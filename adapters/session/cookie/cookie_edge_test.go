package cookie

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestNew_valueBoundaries covers empty and very large session IDs: the value
// is passed through verbatim with no truncation.
func TestNew_valueBoundaries(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("a", 1<<20)

	tests := []struct {
		name  string
		value string
	}{
		{"empty", ""},
		{"single", "x"},
		{"long", long},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := New(tt.value, Options{})
			if c.Value != tt.value {
				t.Fatalf("Value length = %d, want %d", len(c.Value), len(tt.value))
			}
		})
	}
}

// TestNew_emptyNameAndPath_defaulted verifies an explicitly empty Name/Path
// still falls back to the secure defaults rather than emitting blanks.
func TestNew_emptyNameAndPath_defaulted(t *testing.T) {
	t.Parallel()

	c := New("sess-id", Options{Name: "", Path: ""})

	if c.Name != DefaultName {
		t.Fatalf("Name = %q, want %q", c.Name, DefaultName)
	}

	if c.Path != DefaultPath {
		t.Fatalf("Path = %q, want %q", c.Path, DefaultPath)
	}
}

// TestNew_sameSiteNoneMode_explicit proves an explicit SameSiteNoneMode is
// honored verbatim (New does not rewrite it, even though browsers pair it
// with Secure).
func TestNew_sameSiteNoneMode_explicit(t *testing.T) {
	t.Parallel()

	c := New("sess-id", Options{SameSite: http.SameSiteNoneMode})

	if c.SameSite != http.SameSiteNoneMode {
		t.Fatalf("SameSite = %v, want %v", c.SameSite, http.SameSiteNoneMode)
	}
}

// TestNew_maxAgeBoundaryOne proves the smallest positive MaxAge sets both
// Max-Age and Expires one second out.
func TestNew_maxAgeBoundaryOne(t *testing.T) {
	t.Parallel()

	before := time.Now()
	c := New("sess-id", Options{MaxAge: 1})
	after := time.Now()

	if c.MaxAge != 1 {
		t.Fatalf("MaxAge = %d, want 1", c.MaxAge)
	}
	if c.Expires.Before(before.Add(time.Second)) || c.Expires.After(after.Add(time.Second)) {
		t.Fatalf("Expires = %v, want ~1s out", c.Expires)
	}
}

// TestNew_explicitHTTPOnlyTrue proves an explicit true pointer is respected
// the same as the default.
func TestNew_explicitHTTPOnlyTrue(t *testing.T) {
	t.Parallel()

	httpOnly := true

	c := New("sess-id", Options{HTTPOnly: &httpOnly})
	if !c.HttpOnly {
		t.Fatal("HttpOnly = false, want true")
	}
}

// TestNew_hostPrefixNegativeMaxAge proves a deletion cookie keeps the __Host-
// constraints (Secure, Path=/, no Domain) even when MaxAge is negative.
func TestNew_hostPrefixNegativeMaxAge(t *testing.T) {
	t.Parallel()

	c := New("sess-id", Options{Prefix: PrefixHost, MaxAge: -1})

	if c.Name != "__Host-"+DefaultName {
		t.Fatalf("Name = %q, want %q", c.Name, "__Host-"+DefaultName)
	}
	if !c.Secure {
		t.Fatal("Secure = false, want true (__Host- requires Secure)")
	}
	if c.Path != "/" {
		t.Fatalf("Path = %q, want /", c.Path)
	}
	if c.Domain != "" {
		t.Fatalf("Domain = %q, want empty", c.Domain)
	}
	if c.MaxAge != -1 {
		t.Fatalf("MaxAge = %d, want -1", c.MaxAge)
	}
}

// TestSet_emptySessionID proves Set writes a cookie with an empty value
// rather than skipping the header.
func TestSet_emptySessionID(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	Set(rec, "", Options{})

	res := rec.Result()
	defer res.Body.Close()

	cookies := res.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("got %d cookies, want 1", len(cookies))
	}
	if cookies[0].Value != "" {
		t.Fatalf("cookie value = %q, want empty", cookies[0].Value)
	}
	if cookies[0].Name != DefaultName {
		t.Fatalf("cookie name = %q, want %q", cookies[0].Name, DefaultName)
	}
}
