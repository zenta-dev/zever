package billing

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

type stubBilling struct {
	customer     Customer
	subscription Subscription
	invoice      Invoice
}

func (s *stubBilling) CreateCustomer(_ context.Context, name, email, _ string) (Customer, error) {
	c := s.customer
	c.Name = name
	c.Email = email

	return c, nil
}

func (s *stubBilling) CreateSubscription(_ context.Context, customerID, planID, _ string) (Subscription, error) {
	sub := s.subscription
	sub.CustomerID = customerID
	sub.PlanID = planID

	return sub, nil
}

func (s *stubBilling) CancelSubscription(_ context.Context, _ string) error {
	return nil
}

func (s *stubBilling) GetInvoice(_ context.Context, _ string) (Invoice, error) {
	return s.invoice, nil
}

func (s *stubBilling) Close() error {
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
	ok := func(Options) (Billing, error) { return &stubBilling{}, nil }
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

	if err := Register(a, func(Options) (Billing, error) { return nil, sentinel }); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	got, err := Open(a, Options{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Open err = %v, want wrap of sentinel", err)
	}

	if !strings.Contains(err.Error(), "billing: open") {
		t.Fatalf("Open err %q missing %q", err.Error(), "billing: open")
	}

	if got != nil {
		t.Fatalf("Open error value = %v, want nil", got)
	}
}

func TestOpen_success_smoke(t *testing.T) {
	a := freshAdapter()
	wantCustomer := Customer{ID: "cus_1"}
	wantSub := Subscription{ID: "sub_1", Status: SubscriptionActive}
	wantInvoice := Invoice{ID: "in_1", AmountDue: 1000, Currency: "USD", Status: InvoiceOpen}
	if err := Register(a, func(Options) (Billing, error) {
		return &stubBilling{customer: wantCustomer, subscription: wantSub, invoice: wantInvoice}, nil
	}); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	got, err := Open(a, Options{})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}

	ctx := t.Context()

	cus, err := got.CreateCustomer(ctx, "Ada", "ada@example.com", "")
	if err != nil {
		t.Fatalf("CreateCustomer err = %v", err)
	}

	if cus.ID != wantCustomer.ID {
		t.Fatalf("CreateCustomer = %+v, want ID %q", cus, wantCustomer.ID)
	}

	sub, err := got.CreateSubscription(ctx, cus.ID, "plan_pro", "")
	if err != nil {
		t.Fatalf("CreateSubscription err = %v", err)
	}

	if sub.ID != wantSub.ID {
		t.Fatalf("CreateSubscription = %+v, want ID %q", sub, wantSub.ID)
	}

	if cancelErr := got.CancelSubscription(ctx, sub.ID); cancelErr != nil {
		t.Fatalf("CancelSubscription err = %v", cancelErr)
	}

	inv, err := got.GetInvoice(ctx, cus.ID)
	if err != nil {
		t.Fatalf("GetInvoice err = %v", err)
	}

	if inv != wantInvoice {
		t.Fatalf("GetInvoice = %+v, want %+v", inv, wantInvoice)
	}

	if err := got.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}
}

func TestOpen_invalidOptions_propagatesInvalidOptions(t *testing.T) {
	a := freshAdapter()
	if err := Register(a, func(Options) (Billing, error) { return &stubBilling{}, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	got, err := Open(a, Options{Endpoint: "example.com/hook"})
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
