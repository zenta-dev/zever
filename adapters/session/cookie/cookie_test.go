package cookie

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNew_defaultConfig_isSecureByDefault(t *testing.T) {
	t.Parallel()

	c := New("sess-id", Options{})

	if c.Value != "sess-id" {
		t.Fatalf("Value = %q, want %q", c.Value, "sess-id")
	}
	if c.Name != DefaultName {
		t.Fatalf("Name = %q, want %q", c.Name, DefaultName)
	}
	if c.Path != "/" {
		t.Fatalf("Path = %q, want %q", c.Path, "/")
	}
	if !c.Secure {
		t.Fatal("Secure = false, want true by default")
	}
	if !c.HttpOnly {
		t.Fatal("HttpOnly = false, want true by default")
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Fatalf("SameSite = %v, want %v", c.SameSite, http.SameSiteLaxMode)
	}
}

func TestNew_explicitOverrides_areRespected(t *testing.T) {
	t.Parallel()

	insecure := false
	notHTTPOnly := false

	c := New("sess-id", Options{
		Name:     "sid",
		Path:     "/app",
		Domain:   "example.com",
		Secure:   &insecure,
		HTTPOnly: &notHTTPOnly,
		SameSite: http.SameSiteStrictMode,
	})

	if c.Name != "sid" {
		t.Fatalf("Name = %q, want %q", c.Name, "sid")
	}
	if c.Path != "/app" {
		t.Fatalf("Path = %q, want %q", c.Path, "/app")
	}
	if c.Domain != "example.com" {
		t.Fatalf("Domain = %q, want %q", c.Domain, "example.com")
	}
	if c.Secure {
		t.Fatal("Secure = true, want false override to be respected")
	}
	if c.HttpOnly {
		t.Fatal("HttpOnly = true, want false override to be respected")
	}
	if c.SameSite != http.SameSiteStrictMode {
		t.Fatalf("SameSite = %v, want %v", c.SameSite, http.SameSiteStrictMode)
	}
}

func TestNew_explicitSecureTrue_isRespected(t *testing.T) {
	t.Parallel()

	secure := true

	c := New("sess-id", Options{Secure: &secure})
	if !c.Secure {
		t.Fatal("Secure = false, want explicit true to be respected")
	}
}

func TestNew_zeroMaxAge_isSessionCookie(t *testing.T) {
	t.Parallel()

	c := New("sess-id", Options{})

	if c.MaxAge != 0 {
		t.Fatalf("MaxAge = %d, want 0 (session cookie)", c.MaxAge)
	}
	if !c.Expires.IsZero() {
		t.Fatalf("Expires = %v, want zero value for a session cookie", c.Expires)
	}
}

func TestNew_explicitMaxAge_setsExpires(t *testing.T) {
	t.Parallel()

	before := time.Now()
	c := New("sess-id", Options{MaxAge: 3600})
	after := time.Now()

	if c.MaxAge != 3600 {
		t.Fatalf("MaxAge = %d, want 3600", c.MaxAge)
	}
	if c.Expires.IsZero() {
		t.Fatal("Expires is zero, want it set for a non-zero MaxAge")
	}

	wantMin := before.Add(3600 * time.Second)
	wantMax := after.Add(3600 * time.Second)
	if c.Expires.Before(wantMin) || c.Expires.After(wantMax) {
		t.Fatalf("Expires = %v, want between %v and %v", c.Expires, wantMin, wantMax)
	}
}

func TestNew_negativeMaxAge_deletesImmediately(t *testing.T) {
	t.Parallel()

	c := New("sess-id", Options{MaxAge: -1})

	if c.MaxAge != -1 {
		t.Fatalf("MaxAge = %d, want -1", c.MaxAge)
	}
	if c.Expires.IsZero() {
		t.Fatal("Expires is zero, want it set (in the past) for a negative MaxAge")
	}
	if !c.Expires.Before(time.Now()) {
		t.Fatalf("Expires = %v, want a time in the past", c.Expires)
	}
}

func TestSet_writesSetCookieHeader(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	Set(rec, "sess-id", Options{})

	res := rec.Result()
	defer res.Body.Close()

	cookies := res.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("got %d cookies, want 1", len(cookies))
	}
	if cookies[0].Name != DefaultName {
		t.Fatalf("cookie name = %q, want %q", cookies[0].Name, DefaultName)
	}
	if cookies[0].Value != "sess-id" {
		t.Fatalf("cookie value = %q, want %q", cookies[0].Value, "sess-id")
	}
}
