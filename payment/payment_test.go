package payment

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
)

var testSeq atomic.Int64

func freshAdapter() Adapter {
	return Adapter(1000 + int(testSeq.Add(1)))
}

type stubPayment struct {
	result Result
}

func (s *stubPayment) CreatePayment(_ context.Context, _ Request) (Result, error) {
	return s.result, nil
}

func (s *stubPayment) Refund(_ context.Context, _ string, _ int64) error {
	return nil
}

func (s *stubPayment) GetPayment(_ context.Context, _ string) (Result, error) {
	return s.result, nil
}

func (s *stubPayment) WebhookEvent(_ context.Context, _ []byte, _ string) (Event, error) {
	return Event{Type: EventType("stub.event"), Object: s.result}, nil
}

func (s *stubPayment) Close() error {
	return nil
}

func TestRegister_nilFactory_returnsNilFactory(t *testing.T) {
	a := freshAdapter()
	if err := Register(a, nil); !errors.Is(err, ErrNilFactory) {
		t.Fatalf("Register nil err = %v, want ErrNilFactory", err)
	}
}

func TestRegister_duplicate_returnsDuplicateError(t *testing.T) {
	a := freshAdapter()
	ok := func(Options) (Payment, error) { return &stubPayment{}, nil }
	if err := Register(a, ok); err != nil {
		t.Fatalf("first Register err = %v", err)
	}

	if err := Register(a, ok); !errors.Is(err, ErrDuplicateAdapter) {
		t.Fatalf("second Register err = %v, want ErrDuplicateAdapter", err)
	}

	var de *DuplicateAdapterError

	if err := Register(a, ok); !errors.As(err, &de) {
		t.Fatalf("dup err %T is not *DuplicateAdapterError", err)
	} else if de.Adapter != a {
		t.Fatalf("dup carried adapter = %v, want %v", de.Adapter, a)
	}
}

func TestOpen_unknownAdapter_returnsUnknownAndNil(t *testing.T) {
	a := Adapter(9999)

	got, err := Open(a, Options{})
	if !errors.Is(err, ErrUnknownAdapter) {
		t.Fatalf("Open unknown err = %v, want ErrUnknownAdapter", err)
	}

	var ue *UnknownAdapterError
	if !errors.As(err, &ue) {
		t.Fatalf("err %T is not *UnknownAdapterError", err)
	}

	if ue.Adapter != a {
		t.Fatalf("carried adapter = %v, want %v", ue.Adapter, a)
	}

	if got != nil {
		t.Fatalf("Open unknown value = %v, want nil", got)
	}
}

func TestOpen_factoryError_wrappedWithPrefixAndNil(t *testing.T) {
	a := freshAdapter()
	sentinel := errors.New("boom")

	if err := Register(a, func(Options) (Payment, error) { return nil, sentinel }); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	got, err := Open(a, Options{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Open err = %v, want wrap of sentinel", err)
	}

	if !strings.Contains(err.Error(), "payment: open") {
		t.Fatalf("Open err %q missing %q", err.Error(), "payment: open")
	}

	if got != nil {
		t.Fatalf("Open error value = %v, want nil", got)
	}
}

func TestOpen_success_smoke(t *testing.T) {
	a := freshAdapter()
	want := Result{ID: "pay_1", Status: PaymentSucceeded, Amount: 100, Currency: "USD"}
	if err := Register(a, func(Options) (Payment, error) { return &stubPayment{result: want}, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	got, err := Open(a, Options{})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}

	ctx := context.Background()

	res, err := got.CreatePayment(ctx, Request{Amount: 100, Currency: "USD", Method: MethodCard})
	if err != nil {
		t.Fatalf("CreatePayment err = %v", err)
	}

	if res != want {
		t.Fatalf("CreatePayment = %+v, want %+v", res, want)
	}

	if err := got.Refund(ctx, "pay_1", 100); err != nil {
		t.Fatalf("Refund err = %v", err)
	}

	if res, err := got.GetPayment(ctx, "pay_1"); err != nil {
		t.Fatalf("GetPayment err = %v", err)
	} else if res != want {
		t.Fatalf("GetPayment = %+v, want %+v", res, want)
	}

	if _, err := got.WebhookEvent(ctx, []byte(`{}`), "sig"); err != nil {
		t.Fatalf("WebhookEvent err = %v", err)
	}

	if err := got.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}
}

func TestOpen_invalidOptions_propagatesInvalidOptions(t *testing.T) {
	a := freshAdapter()
	if err := Register(a, func(Options) (Payment, error) { return &stubPayment{}, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	got, err := Open(a, Options{MaxWebhookBytes: -1})
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("Open err = %v, want ErrInvalidOptions", err)
	}

	var ioe *InvalidOptionsError
	if !errors.As(err, &ioe) {
		t.Fatalf("err %T is not *InvalidOptionsError", err)
	}

	if got != nil {
		t.Fatalf("Open error value = %v, want nil", got)
	}
}
