package eventbus

import (
	"errors"
	"strings"
	"testing"
)

func TestCoverTypedErrorStrings(t *testing.T) {
	dup := DuplicateError{Adapter: Memory}
	unknown := UnknownAdapterError{Adapter: Redis}
	invalidAdapter := InvalidAdapterError{Adapter: "bogus"}
	invalidOpts := InvalidOptionsError{Reason: "bad"}
	withCause := InvalidMessageIDError{ID: "x", Err: errors.New("boom")}
	nilCause := InvalidMessageIDError{ID: "x"}

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"DuplicateError", dup.Error(), "eventbus: duplicate registration: memory"},
		{"UnknownAdapterError", unknown.Error(), "eventbus: unknown adapter: redis (forgotten import?)"},
		{"InvalidAdapterError", invalidAdapter.Error(), `eventbus: invalid adapter: "bogus"`},
		{"InvalidOptionsError", invalidOpts.Error(), "eventbus: invalid options: bad"},
		{"InvalidMessageIDErrorWithCause", withCause.Error(), `eventbus: invalid message id "x": boom`},
		{"InvalidMessageIDErrorNilCause", nilCause.Error(), `eventbus: invalid message id "x": <nil>`},
	}

	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %q want %q", c.name, c.got, c.want)
		}
	}
}

func TestCoverInvalidMessageIDUnwrapBothArms(t *testing.T) {
	cause := errors.New("boom")

	withCause := (&InvalidMessageIDError{ID: "x", Err: cause}).Unwrap()
	if len(withCause) != 2 {
		t.Fatalf("Unwrap with cause len = %d want 2", len(withCause))
	}

	if !errors.Is(withCause[0], ErrInvalidMessageID) || !errors.Is(withCause[1], cause) {
		t.Errorf("Unwrap with cause = %v want [ErrInvalidMessageID cause]", withCause)
	}

	withoutCause := (&InvalidMessageIDError{ID: "x"}).Unwrap()
	if len(withoutCause) != 1 {
		t.Fatalf("Unwrap without cause len = %d want 1", len(withoutCause))
	}

	if !errors.Is(withoutCause[0], ErrInvalidMessageID) {
		t.Errorf("Unwrap without cause = %v want [ErrInvalidMessageID]", withoutCause)
	}
}

func TestCoverOpenSuccess(t *testing.T) {
	a := freshAdapter()
	if regErr := Register(a, func(Options) (EventBus, error) { return stubBus{}, nil }); regErr != nil {
		t.Fatalf("Register: %v", regErr)
	}

	b, openErr := Open(a, Options{})
	if openErr != nil {
		t.Fatalf("Open: %v", openErr)
	}

	if name := b.Name(); name != "stub" {
		t.Errorf("Name = %q want stub", name)
	}

	if closeErr := b.Close(); closeErr != nil {
		t.Fatalf("Close: %v", closeErr)
	}
}

func TestCoverOpenFactoryErrorWrap(t *testing.T) {
	a := freshAdapter()
	sentinel := errors.New("cover boom")

	if regErr := Register(a, func(Options) (EventBus, error) { return nil, sentinel }); regErr != nil {
		t.Fatalf("Register: %v", regErr)
	}

	_, openErr := Open(a, Options{})
	if !errors.Is(openErr, sentinel) {
		t.Fatalf("Open err = %v want wrap of sentinel", openErr)
	}

	if !strings.Contains(openErr.Error(), "eventbus: open") {
		t.Errorf("Open err %q missing %q", openErr.Error(), "eventbus: open")
	}
}
