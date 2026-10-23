package eventbus

import (
	"errors"
	"testing"
)

func TestCoverInvalidMessageIDUnwrapBothArms(t *testing.T) {
	cause := errors.New("boom")

	withCause := (InvalidMessageIDError{ID: "x", Err: cause}).Unwrap()
	if !errors.Is(withCause, ErrInvalidMessageID) {
		t.Errorf("Unwrap with cause = %v want ErrInvalidMessageID", withCause)
	}

	if !errors.Is(withCause, cause) {
		t.Errorf("Unwrap with cause = %v want cause", withCause)
	}

	withoutCause := (InvalidMessageIDError{ID: "x"}).Unwrap()
	if !errors.Is(withoutCause, ErrInvalidMessageID) {
		t.Errorf("Unwrap without cause = %v want ErrInvalidMessageID", withoutCause)
	}

	if errors.Is(withoutCause, cause) {
		t.Errorf("Unwrap without cause = %v want no cause", withoutCause)
	}
}
