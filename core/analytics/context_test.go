package analytics

import (
	"context"
	"testing"
)

func TestUserID_missing_returnsEmpty(t *testing.T) {
	t.Parallel()
	if got := UserID(t.Context()); got != "" {
		t.Errorf("UserID() = %q, want %q", got, "")
	}
}

func TestWithUserID_roundtrip_returnsID(t *testing.T) {
	t.Parallel()
	ctx := WithUserID(t.Context(), "u1")
	if got := UserID(ctx); got != "u1" {
		t.Errorf("UserID() = %q, want %q", got, "u1")
	}
}

func TestUserID_wrongType_returnsEmpty(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(t.Context(), userIDKey{}, 123)
	if got := UserID(ctx); got != "" {
		t.Errorf("UserID() = %q, want %q", got, "")
	}
}

func TestUserIDWithFallback_branches(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		userID   string
		present  bool
		fallback string
		want     string
	}{
		{"present", "u1", true, "anon", "u1"},
		{"missing", "", false, "anon", "anon"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			if tc.present {
				ctx = WithUserID(ctx, tc.userID)
			}
			if got := UserIDWithFallback(ctx, tc.fallback); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
