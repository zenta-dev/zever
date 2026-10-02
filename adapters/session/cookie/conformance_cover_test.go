package cookie_test

import (
	"testing"
	"time"

	"github.com/zenta-dev/zever/adapters/session/cookie"
)

// TestConformanceMaxAge covers the session TTL to cookie MaxAge mapping:
// MaxAge is int-seconds (net/http shape, the deliberate exception to the
// time.Duration rule) mirroring the store TTL span as a client lifetime.
// Zero stays a session cookie; negative deletes immediately.
func TestConformanceMaxAge(t *testing.T) {
	t.Parallel()

	before := time.Now()
	c := cookie.New("sid", cookie.Options{MaxAge: 90})

	if c.MaxAge != 90 {
		t.Errorf("MaxAge = %d, want 90 (int-seconds preserved)", c.MaxAge)
	}

	if want := before.Add(90 * time.Second); c.Expires.Before(want.Add(-time.Minute)) || c.Expires.After(want.Add(time.Minute)) {
		t.Errorf("Expires = %v, want ~%v", c.Expires, want)
	}

	sess := cookie.New("sid", cookie.Options{})
	if sess.MaxAge != 0 {
		t.Errorf("MaxAge = %d, want 0 (session cookie)", sess.MaxAge)
	}

	if !sess.Expires.IsZero() {
		t.Errorf("Expires = %v, want zero (no Max-Age/Expires sent)", sess.Expires)
	}

	del := cookie.New("sid", cookie.Options{MaxAge: -1})
	if del.MaxAge >= 0 {
		t.Errorf("MaxAge = %d, want negative (immediate delete)", del.MaxAge)
	}
}
