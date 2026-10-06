package passwordtest

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/password"
)

// stubHasher is a scriptable password.Hasher for proving the check*
// helpers catch broken adapters. Zero value verifies nothing.
type stubHasher struct {
	onHash     func(ctx context.Context, secret string) (string, error)
	onVerify   func(ctx context.Context, hash, secret string) (bool, error)
	onRehash   func(ctx context.Context, hash string) (bool, error)
	hashSeq    []string
	verifySeq  []verifyResult
	verifyCall int
}

type verifyResult struct {
	ok  bool
	err error
}

func (s *stubHasher) Hash(ctx context.Context, secret string) (string, error) {
	if s.onHash != nil {
		return s.onHash(ctx, secret)
	}
	if len(s.hashSeq) == 0 {
		return "", errors.New("stubHasher: no scripted hash")
	}
	out := s.hashSeq[0]
	s.hashSeq = s.hashSeq[1:]
	return out, nil
}

func (s *stubHasher) Verify(ctx context.Context, hash, secret string) (bool, error) {
	if s.onVerify != nil {
		return s.onVerify(ctx, hash, secret)
	}
	if s.verifyCall >= len(s.verifySeq) {
		return false, errors.New("stubHasher: no scripted verify")
	}
	r := s.verifySeq[s.verifyCall]
	s.verifyCall++
	return r.ok, r.err
}

func (s *stubHasher) NeedsRehash(ctx context.Context, hash string) (bool, error) {
	if s.onRehash != nil {
		return s.onRehash(ctx, hash)
	}
	return false, password.ErrInvalidHash
}

var _ password.Hasher = (*stubHasher)(nil)

func TestCheckHashVerify_Negatives(t *testing.T) {
	t.Parallel()

	boom := errors.New("hash boom")
	tests := []struct {
		name string
		stub *stubHasher
		want string
	}{
		{name: "hash error", stub: &stubHasher{onHash: func(context.Context, string) (string, error) { return "", boom }}, want: "Hash() error"},
		{name: "empty hash", stub: &stubHasher{hashSeq: []string{""}}, want: "want opaque encoded hash"},
		{name: "plaintext echo", stub: &stubHasher{hashSeq: []string{"conformance-password-01"}}, want: "want opaque encoded hash"},
		{name: "second hash error", stub: &stubHasher{onHash: firstThenError("h1", boom)}, want: "Hash() error"},
		{name: "identical salts", stub: &stubHasher{hashSeq: []string{"same", "same"}, verifySeq: []verifyResult{{ok: true}, {ok: true}}}, want: "identical hashes"},
		{name: "verify error", stub: &stubHasher{hashSeq: []string{"h1", "h2"}, verifySeq: []verifyResult{{ok: false, err: boom}}}, want: "Verify() error"},
		{name: "verify false", stub: &stubHasher{hashSeq: []string{"h1", "h2"}, verifySeq: []verifyResult{{ok: false}}}, want: "Verify(correct) = false"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := checkHashVerify(t.Context(), tt.stub); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("checkHashVerify() = %v, want containing %q", err, tt.want)
			}
		})
	}
}

// firstThenError serves one scripted hash, then fails every later Hash call.
func firstThenError(first string, err error) func(context.Context, string) (string, error) {
	called := false
	return func(context.Context, string) (string, error) {
		if !called {
			called = true
			return first, nil
		}
		return "", err
	}
}

func TestCheckWrongPassword_Negatives(t *testing.T) {
	t.Parallel()

	boom := errors.New("verify boom")
	okStub := &stubHasher{
		hashSeq:   []string{"h"},
		verifySeq: []verifyResult{{ok: true}, {ok: false}},
	}
	emptyOK := &stubHasher{
		hashSeq:   []string{"h"},
		verifySeq: []verifyResult{{ok: false}, {ok: true}},
	}
	tests := []struct {
		name string
		stub *stubHasher
		want string
	}{
		{name: "hash error", stub: &stubHasher{onHash: func(context.Context, string) (string, error) { return "", boom }}, want: "Hash() error"},
		{name: "wrong verify error", stub: &stubHasher{hashSeq: []string{"h"}, verifySeq: []verifyResult{{ok: false, err: boom}}}, want: "Verify(wrong) error"},
		{name: "wrong accepted", stub: okStub, want: "Verify(wrong) = true"},
		{name: "empty verify error", stub: &stubHasher{hashSeq: []string{"h"}, verifySeq: []verifyResult{{ok: false}, {ok: false, err: boom}}}, want: "Verify(empty) error"},
		{name: "empty accepted", stub: emptyOK, want: "Verify(empty) = true"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := checkWrongPassword(t.Context(), tt.stub); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("checkWrongPassword() = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestCheckNeedsRehash_Negatives(t *testing.T) {
	t.Parallel()

	boom := errors.New("rehash boom")
	tests := []struct {
		name string
		stub *stubHasher
		want string
	}{
		{name: "hash error", stub: &stubHasher{onHash: func(context.Context, string) (string, error) { return "", boom }}, want: "Hash() error"},
		{name: "rehash error", stub: &stubHasher{hashSeq: []string{"h"}, onRehash: func(context.Context, string) (bool, error) { return false, boom }}, want: "NeedsRehash(current) error"},
		{name: "needs rehash", stub: &stubHasher{hashSeq: []string{"h"}, onRehash: func(context.Context, string) (bool, error) { return true, nil }}, want: "NeedsRehash(current) = true"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := checkNeedsRehash(t.Context(), tt.stub); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("checkNeedsRehash() = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestCheckInvalidHash_Negatives(t *testing.T) {
	t.Parallel()

	boom := errors.New("backend boom")
	acceptAll := &stubHasher{
		onVerify: func(context.Context, string, string) (bool, error) { return false, nil },
		onRehash: func(context.Context, string) (bool, error) { return false, nil },
		onHash:   func(context.Context, string) (string, error) { return "h", nil },
	}
	verifyBoom := &stubHasher{
		onVerify: func(context.Context, string, string) (bool, error) { return false, boom },
		onRehash: func(context.Context, string) (bool, error) { return false, password.ErrInvalidHash },
	}
	rehashBoom := &stubHasher{
		onVerify: func(context.Context, string, string) (bool, error) { return false, password.ErrInvalidHash },
		onRehash: func(context.Context, string) (bool, error) { return false, boom },
	}
	rehashAccept := &stubHasher{
		onVerify: func(context.Context, string, string) (bool, error) { return false, password.ErrInvalidHash },
		onRehash: func(context.Context, string) (bool, error) { return false, nil },
	}
	wrongOverlong := &stubHasher{
		onVerify: func(context.Context, string, string) (bool, error) { return false, password.ErrInvalidHash },
		onRehash: func(context.Context, string) (bool, error) { return false, password.ErrInvalidHash },
		onHash:   func(context.Context, string) (string, error) { return "", boom },
	}
	tests := []struct {
		name string
		stub *stubHasher
		want string
	}{
		{name: "verify accepts garbage", stub: acceptAll, want: "Verify(\"\") = nil"},
		{name: "verify backend error", stub: verifyBoom, want: "Verify(\"\") err = backend boom"},
		{name: "rehash backend error", stub: rehashBoom, want: "NeedsRehash(\"\") err = backend boom"},
		{name: "rehash accepts garbage", stub: rehashAccept, want: "NeedsRehash(\"\") = nil"},
		{name: "overlong wrong error", stub: wrongOverlong, want: "Hash(overlong) err = backend boom"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := checkInvalidHash(t.Context(), tt.stub); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("checkInvalidHash() = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestCheckInvalidHash_OverlongAccepted(t *testing.T) {
	t.Parallel()

	stub := &stubHasher{
		onVerify: func(context.Context, string, string) (bool, error) { return false, password.ErrInvalidHash },
		onRehash: func(context.Context, string) (bool, error) { return false, password.ErrInvalidHash },
		onHash:   func(context.Context, string) (string, error) { return "h", nil },
	}
	if err := checkInvalidHash(t.Context(), stub); err == nil || !strings.Contains(err.Error(), "Hash(overlong) = nil") {
		t.Errorf("checkInvalidHash() = %v, want overlong rejection", err)
	}
}
