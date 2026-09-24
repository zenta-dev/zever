package session_test

import (
	"errors"
	"testing"
	"time"

	"github.com/zenta-dev/zever/auth"
	authsession "github.com/zenta-dev/zever/auth/session"
	"github.com/zenta-dev/zever/session"
	sessionmemory "github.com/zenta-dev/zever/session/memory"
)

func newAdapter(t *testing.T, store session.Store) auth.Auth {
	t.Helper()
	a, err := authsession.New(auth.Options{Session: auth.SessionOptions{Store: store}})
	if err != nil {
		t.Fatalf("New err = %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })
	return a
}

func newMemoryStore(t *testing.T) session.Store {
	t.Helper()
	st, err := sessionmemory.New(session.Options{})
	if err != nil {
		t.Fatalf("memory.New err = %v", err)
	}
	return st
}

func eventually(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", msg)
		}

		time.Sleep(5 * time.Millisecond)
	}
}

// plant hand-writes a session record with raw Data, bypassing Issue.
func plant(t *testing.T, st session.Store, data map[string]any) string {
	t.Helper()
	id := session.NewID()
	sess := session.NewSession(id, time.Hour)
	sess.Data = data
	if err := st.Save(t.Context(), sess); err != nil {
		t.Fatalf("Save plant err = %v", err)
	}
	return id
}

func TestRoundtrip(t *testing.T) {
	t.Parallel()
	a := newAdapter(t, newMemoryStore(t))
	ctx := t.Context()

	custom := map[string]any{"role": "admin", "tags": []string{"a", "b"}}
	tok, err := a.Issue(ctx, "alice", custom, time.Hour)
	if err != nil {
		t.Fatalf("Issue err = %v", err)
	}
	if tok.Value == "" {
		t.Fatal("Issue returned empty token value")
	}
	if verr := session.ValidateID(tok.Value); verr != nil {
		t.Fatalf("token value not a valid session ID: %v", verr)
	}
	if tok.ExpiresAt.IsZero() {
		t.Fatal("Issue returned zero ExpiresAt")
	}

	got, err := a.Verify(ctx, tok.Value)
	if err != nil {
		t.Fatalf("Verify err = %v", err)
	}
	if got.Subject != "alice" {
		t.Fatalf("Subject = %q, want alice", got.Subject)
	}
	if got.Custom["role"] != "admin" {
		t.Fatalf("Custom[role] = %v, want admin", got.Custom["role"])
	}
	// Envelope stores exp at second precision; compare accordingly.
	if got.ExpiresAt.Unix() != tok.ExpiresAt.Unix() {
		t.Fatalf("Claims.ExpiresAt = %v, want %v", got.ExpiresAt, tok.ExpiresAt)
	}
}

func TestNilStoreDefault(t *testing.T) {
	t.Parallel()
	a, err := authsession.New(auth.Options{})
	if err != nil {
		t.Fatalf("New(zero opts) err = %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })

	ctx := t.Context()
	tok, err := a.Issue(ctx, "bob", nil, time.Minute)
	if err != nil {
		t.Fatalf("Issue err = %v", err)
	}
	got, err := a.Verify(ctx, tok.Value)
	if err != nil {
		t.Fatalf("Verify err = %v", err)
	}
	if got.Subject != "bob" {
		t.Fatalf("Subject = %q, want bob", got.Subject)
	}
}

func TestInvalidOptions(t *testing.T) {
	t.Parallel()
	_, err := authsession.New(auth.Options{JWT: auth.JWTOptions{MaxTTL: -time.Second}})
	if err == nil {
		t.Fatal("New(invalid opts) = nil, want error")
	}
}

func TestIssueBadInputs(t *testing.T) {
	t.Parallel()
	a := newAdapter(t, newMemoryStore(t))
	ctx := t.Context()

	if _, err := a.Issue(ctx, "", nil, time.Minute); !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("Issue(empty subject) err = %v, want ErrInvalidToken", err)
	}
	if _, err := a.Issue(ctx, "u", nil, 0); err == nil {
		t.Fatal("Issue(ttl=0) = nil, want error")
	}
	if _, err := a.Issue(ctx, "u", nil, -time.Second); err == nil {
		t.Fatal("Issue(ttl<0) = nil, want error")
	}
}

func TestVerifyUnknown(t *testing.T) {
	t.Parallel()
	a := newAdapter(t, newMemoryStore(t))

	if _, err := a.Verify(t.Context(), session.NewID()); !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("Verify(unknown) err = %v, want ErrInvalidToken", err)
	}
}

func TestVerifyBadInputs(t *testing.T) {
	t.Parallel()
	a := newAdapter(t, newMemoryStore(t))
	ctx := t.Context()

	if _, err := a.Verify(ctx, ""); !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("Verify(empty) err = %v, want ErrInvalidToken", err)
	}
	if _, err := a.Verify(ctx, "not-a-valid-id"); err == nil {
		t.Fatal("Verify(malformed) = nil, want error")
	}
	if _, err := a.Verify(ctx, "short"); err == nil {
		t.Fatal("Verify(short) = nil, want error")
	}
}

func TestVerifyCorruptEnvelope(t *testing.T) {
	t.Parallel()
	st := newMemoryStore(t)
	a := newAdapter(t, st)
	ctx := t.Context()

	now := time.Now().Unix()
	cases := map[string]map[string]any{
		"empty":        {},
		"missing exp":  {"sub": "u", "custom": map[string]any{}},
		"missing sub":  {"custom": map[string]any{}, "exp": now + 3600},
		"bad sub type": {"sub": 123, "custom": map[string]any{}, "exp": now + 3600},
		"bad exp type": {"sub": "u", "custom": map[string]any{}, "exp": "tomorrow"},
		"bad custom":   {"sub": "u", "custom": "nope", "exp": now + 3600},
		"empty sub":    {"sub": "", "custom": map[string]any{}, "exp": now + 3600},
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			id := plant(t, st, data)
			if _, err := a.Verify(ctx, id); !errors.Is(err, auth.ErrInvalidToken) {
				t.Fatalf("Verify(corrupt %s) err = %v, want ErrInvalidToken", name, err)
			}
		})
	}
}

func TestVerifyStoreExpired(t *testing.T) {
	t.Parallel()
	a := newAdapter(t, newMemoryStore(t))
	ctx := t.Context()

	tok, err := a.Issue(ctx, "u", nil, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("Issue err = %v", err)
	}
	// Session record itself expires: poll until Verify reports unknown.
	eventually(t, 2*time.Second, func() bool {
		_, err := a.Verify(ctx, tok.Value)
		return errors.Is(err, auth.ErrInvalidToken)
	}, "session record expiry")
}

func TestVerifyEnvelopeExpired(t *testing.T) {
	t.Parallel()
	st := newMemoryStore(t)
	a := newAdapter(t, st)
	ctx := t.Context()

	// Envelope exp past the 1m leeway, session record still live.
	id := plant(t, st, map[string]any{
		"sub": "u", "custom": map[string]any{},
		"exp": time.Now().Add(-2 * time.Minute).Unix(),
	})
	if _, err := a.Verify(ctx, id); !errors.Is(err, auth.ErrTokenExpired) {
		t.Fatalf("Verify(expired envelope) err = %v, want ErrTokenExpired", err)
	}
	// Lazy delete: second read looks unknown.
	if _, err := a.Verify(ctx, id); !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("Verify(after lazy delete) err = %v, want ErrInvalidToken", err)
	}
}

func TestVerifyLeeway(t *testing.T) {
	t.Parallel()
	st := newMemoryStore(t)
	a := newAdapter(t, st)
	ctx := t.Context()

	// Envelope exp 30s ago: inside the 1m clock-skew leeway, still valid.
	id := plant(t, st, map[string]any{
		"sub": "u", "custom": map[string]any{},
		"exp": time.Now().Add(-30 * time.Second).Unix(),
	})
	got, err := a.Verify(ctx, id)
	if err != nil {
		t.Fatalf("Verify(inside leeway) err = %v", err)
	}
	if got.Subject != "u" {
		t.Fatalf("Subject = %q, want u", got.Subject)
	}
}

func TestRevoke(t *testing.T) {
	t.Parallel()
	a := newAdapter(t, newMemoryStore(t))
	ctx := t.Context()

	tok, err := a.Issue(ctx, "u", nil, time.Hour)
	if err != nil {
		t.Fatalf("Issue err = %v", err)
	}
	if err := a.Revoke(ctx, tok.Value); err != nil {
		t.Fatalf("Revoke err = %v", err)
	}
	if _, err := a.Verify(ctx, tok.Value); !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("Verify(after revoke) err = %v, want ErrInvalidToken", err)
	}
	// Idempotent: second revoke is nil (store contract).
	if err := a.Revoke(ctx, tok.Value); err != nil {
		t.Fatalf("Revoke(idempotent) err = %v", err)
	}
}

func TestRevokeBadInputs(t *testing.T) {
	t.Parallel()
	a := newAdapter(t, newMemoryStore(t))
	ctx := t.Context()

	if err := a.Revoke(ctx, ""); err == nil {
		t.Fatal("Revoke(empty) = nil, want error")
	}
	if err := a.Revoke(ctx, "malformed"); err == nil {
		t.Fatal("Revoke(malformed) = nil, want error")
	}
	// Well-shaped but unknown: nil per idempotent store contract.
	if err := a.Revoke(ctx, session.NewID()); err != nil {
		t.Fatalf("Revoke(unknown) err = %v, want nil", err)
	}
}

func TestCustomCloneIndependence(t *testing.T) {
	t.Parallel()
	a := newAdapter(t, newMemoryStore(t))
	ctx := t.Context()

	custom := map[string]any{
		"nested": map[string]any{"k": "v"},
		"list":   []string{"x"},
	}
	tok, err := a.Issue(ctx, "u", custom, time.Hour)
	if err != nil {
		t.Fatalf("Issue err = %v", err)
	}
	// Mutate caller map after Issue: stored copy unaffected.
	nested, ok := custom["nested"].(map[string]any)
	if !ok {
		t.Fatalf("custom nested type = %T, want map[string]any", custom["nested"])
	}
	nested["k"] = "MUTATED"
	list, ok := custom["list"].([]string)
	if !ok {
		t.Fatalf("custom list type = %T, want []string", custom["list"])
	}
	list[0] = "MUTATED"

	got, err := a.Verify(ctx, tok.Value)
	if err != nil {
		t.Fatalf("Verify err = %v", err)
	}
	gotNested, ok := got.Custom["nested"].(map[string]any)
	if !ok || gotNested["k"] != "v" {
		t.Fatalf("stored custom mutated via caller map: %v", got.Custom)
	}
	// Mutate returned claims: next read unaffected.
	gotNested["k"] = "MUTATED2"
	again, err := a.Verify(ctx, tok.Value)
	if err != nil {
		t.Fatalf("Verify err = %v", err)
	}
	againNested, ok := again.Custom["nested"].(map[string]any)
	if !ok || againNested["k"] != "v" {
		t.Fatalf("stored custom mutated via returned claims: %v", again.Custom)
	}
}

func TestClose(t *testing.T) {
	t.Parallel()
	st := newMemoryStore(t)
	a := newAdapter(t, st)

	// newAdapter registers Close via Cleanup; explicit Close must be nil.
	if err := a.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}
}
