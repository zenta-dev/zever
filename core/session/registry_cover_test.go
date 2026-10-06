package session

import (
	"errors"
	"testing"
	"time"
)

func TestOpen_invalidOptions(t *testing.T) {
	t.Parallel()

	store, err := Open(freshAdapter(), Options{TTL: -time.Second})
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("Open(negative ttl) err = %v, want ErrInvalidOptions", err)
	}

	var ioe InvalidOptionsError
	if !errors.As(err, &ioe) {
		t.Fatalf("err %T is not InvalidOptionsError", err)
	}

	if store != nil {
		t.Fatalf("Open(negative ttl) store = %v, want nil", store)
	}
}

func TestOpenShared_invalidOptions(t *testing.T) {
	t.Parallel()

	store, err := OpenShared(freshAdapter(), nil, Options{TTL: -time.Second})
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("OpenShared(negative ttl) err = %v, want ErrInvalidOptions", err)
	}

	var ioe InvalidOptionsError
	if !errors.As(err, &ioe) {
		t.Fatalf("err %T is not InvalidOptionsError", err)
	}

	if store != nil {
		t.Fatalf("OpenShared(negative ttl) store = %v, want nil", store)
	}
}
