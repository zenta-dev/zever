package session

import (
	"errors"
	"strings"
	"testing"
)

func TestNewID_unique_parseable(t *testing.T) {
	t.Parallel()
	a, b := NewID(), NewID()
	if a == b {
		t.Fatal("NewID produced duplicate IDs")
	}
	if err := ValidateID(a); err != nil {
		t.Fatalf("ValidateID(NewID()) err = %v", err)
	}
	if err := ValidateID(b); err != nil {
		t.Fatalf("ValidateID(NewID()) err = %v", err)
	}
}

func TestValidateID_empty_fails(t *testing.T) {
	t.Parallel()
	err := ValidateID("")
	if err == nil {
		t.Fatal("ValidateID empty expected error, got nil")
	}
	if !errors.Is(err, ErrInvalidID) {
		t.Fatalf("ValidateID empty err = %v, want ErrInvalidID", err)
	}
	var iie *InvalidIDError
	if !errors.As(err, &iie) {
		t.Fatalf("err type = %T, want *InvalidIDError", err)
	}
	if iie.IDLen != 0 {
		t.Fatalf("InvalidIDError.IDLen = %d, want 0", iie.IDLen)
	}
}

func TestValidateID_garbage_fails(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"not-a-uuid", "550e8400-e29b-41d4-a716", "a\nb", strings.Repeat("s", 300)} {
		err := ValidateID(id)
		if !errors.Is(err, ErrInvalidID) {
			t.Fatalf("ValidateID(%q) err = %v, want ErrInvalidID", id, err)
		}
		var iie *InvalidIDError
		if !errors.As(err, &iie) {
			t.Fatalf("ValidateID(%q) err type = %T, want *InvalidIDError", id, err)
		}
		if iie.IDLen != len(id) {
			t.Fatalf("InvalidIDError.IDLen = %d, want %d", iie.IDLen, len(id))
		}
	}
}

func TestValidateID_error_never_echoes_id(t *testing.T) {
	t.Parallel()
	secret := "super-secret-session-id"
	err := ValidateID(secret)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error message echoes session ID: %q", err.Error())
	}
}
