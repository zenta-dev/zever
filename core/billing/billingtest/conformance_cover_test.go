package billingtest

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/core/billing"
)

// stubBilling is a scriptable billing.Billing double. Zero value
// succeeds with zero values; healthyStubBilling returns linked IDs.
// Per-method error fields drive each conformance failure branch.
type stubBilling struct {
	mu                sync.Mutex
	customer          billing.Customer
	subscription      billing.Subscription
	invoice           billing.Invoice
	createCustomerErr error
	createSubErr      error
	cancelErr         error
	cancelAgainErr    error
	getInvoiceErr     error
	closeErr          error
	closeAgainErr     error
	cancels           int
	closes            int
}

func healthyStubBilling() *stubBilling {
	return &stubBilling{
		customer:     billing.Customer{ID: "cus_kit", Name: "Kit User", Email: "kit@example.com"},
		subscription: billing.Subscription{ID: "sub_kit", CustomerID: "cus_kit", PlanID: "plan-kit", Status: billing.SubscriptionActive},
		invoice:      billing.Invoice{ID: "in_kit", AmountDue: 100, Currency: "USD", Status: billing.InvoiceOpen},
	}
}

func (s *stubBilling) CreateCustomer(_ context.Context, _, _, _ string) (billing.Customer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.createCustomerErr != nil {
		return billing.Customer{}, s.createCustomerErr
	}

	return s.customer, nil
}

func (s *stubBilling) CreateSubscription(_ context.Context, _, _, _ string) (billing.Subscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.createSubErr != nil {
		return billing.Subscription{}, s.createSubErr
	}

	return s.subscription, nil
}

func (s *stubBilling) CancelSubscription(_ context.Context, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cancels++
	if s.cancels > 1 {
		return s.cancelAgainErr
	}

	return s.cancelErr
}

func (s *stubBilling) GetInvoice(_ context.Context, _ string) (billing.Invoice, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.getInvoiceErr != nil {
		return billing.Invoice{}, s.getInvoiceErr
	}

	return s.invoice, nil
}

func (s *stubBilling) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closes++
	if s.closes > 1 {
		return s.closeAgainErr
	}

	return s.closeErr
}

func mustPass(t *testing.T, err error) {
	t.Helper()

	if err != nil {
		t.Fatalf("check err = %v, want nil", err)
	}
}

func TestCheckOpenRegister_success(t *testing.T) {
	t.Parallel()

	mustPass(t, checkOpenRegister())
}

func TestCheckLifecycle_success(t *testing.T) {
	t.Parallel()

	mustPass(t, checkLifecycle(t.Context(), healthyStubBilling()))
}

func TestCheckLifecycle_failures(t *testing.T) {
	t.Parallel()

	boom := errors.New("billingtest: boom")

	cases := []struct {
		name     string
		mutate   func(*stubBilling)
		contains string
	}{
		{"customer error", func(s *stubBilling) { s.createCustomerErr = boom }, "CreateCustomer() error"},
		{"empty customer id", func(s *stubBilling) { s.customer.ID = "" }, "CreateCustomer() ID is empty"},
		{"subscription error", func(s *stubBilling) { s.createSubErr = boom }, "CreateSubscription() error"},
		{"empty subscription id", func(s *stubBilling) { s.subscription.ID = "" }, "CreateSubscription() ID is empty"},
		{"customer mismatch", func(s *stubBilling) { s.subscription.CustomerID = "cus_other" }, "Subscription.CustomerID"},
		{"plan mismatch", func(s *stubBilling) { s.subscription.PlanID = "plan_other" }, "Subscription.PlanID"},
		{"invoice error", func(s *stubBilling) { s.getInvoiceErr = boom }, "GetInvoice() error"},
		{"empty invoice id", func(s *stubBilling) { s.invoice.ID = "" }, "GetInvoice() ID is empty"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			stub := healthyStubBilling()
			tc.mutate(stub)

			err := checkLifecycle(t.Context(), stub)
			if err == nil {
				t.Fatalf("checkLifecycle() = nil, want error containing %q", tc.contains)
			}
			if !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("checkLifecycle() = %v, want containing %q", err, tc.contains)
			}
		})
	}
}

func TestCheckLifecycle_customerErrorWraps(t *testing.T) {
	t.Parallel()

	boom := errors.New("billingtest: boom")
	stub := healthyStubBilling()
	stub.createCustomerErr = boom

	if err := checkLifecycle(t.Context(), stub); !errors.Is(err, boom) {
		t.Fatalf("checkLifecycle() = %v, want wrap of boom", err)
	}
}

func TestCheckCancel_success(t *testing.T) {
	t.Parallel()

	mustPass(t, checkCancel(t.Context(), healthyStubBilling()))
}

func TestCheckCancel_secondCancelNotFoundTolerated(t *testing.T) {
	t.Parallel()

	stub := healthyStubBilling()
	stub.cancelAgainErr = billing.NotFoundError{Resource: "subscription", ID: "sub_kit"}

	mustPass(t, checkCancel(t.Context(), stub))
}

func TestCheckCancel_failures(t *testing.T) {
	t.Parallel()

	boom := errors.New("billingtest: boom")

	cases := []struct {
		name     string
		mutate   func(*stubBilling)
		contains string
	}{
		{"customer error", func(s *stubBilling) { s.createCustomerErr = boom }, "CreateCustomer() error"},
		{"subscription error", func(s *stubBilling) { s.createSubErr = boom }, "CreateSubscription() error"},
		{"cancel error", func(s *stubBilling) { s.cancelErr = boom }, "CancelSubscription() error"},
		{"second cancel unexpected", func(s *stubBilling) { s.cancelAgainErr = boom }, "CancelSubscription(again)"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			stub := healthyStubBilling()
			tc.mutate(stub)

			err := checkCancel(t.Context(), stub)
			if err == nil {
				t.Fatalf("checkCancel() = nil, want error containing %q", tc.contains)
			}
			if !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("checkCancel() = %v, want containing %q", err, tc.contains)
			}
		})
	}
}

func TestCheckNotFound_reportsAll(t *testing.T) {
	t.Parallel()

	err := checkNotFound(t.Context(), &stubBilling{})
	if err == nil {
		t.Fatal("checkNotFound(zero stub) = nil, want joined errors")
	}

	for _, want := range []string{"CreateSubscription(unknown customer)", "CancelSubscription(missing)", "GetInvoice(missing)"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("checkNotFound() = %v, want containing %q", err, want)
		}
	}
}

func TestCheckNotFound_notFoundSentinelPasses(t *testing.T) {
	t.Parallel()

	stub := &stubBilling{
		createSubErr:  billing.NotFoundError{Resource: "customer", ID: "cus_missing"},
		cancelErr:     billing.ErrNotFound,
		getInvoiceErr: billing.ErrNotFound,
	}

	mustPass(t, checkNotFound(t.Context(), stub))

	var nf billing.NotFoundError
	if !errors.As(stub.createSubErr, &nf) {
		t.Fatalf("stub err %T is not billing.NotFoundError", stub.createSubErr)
	}
}

func TestCheckMissingIDs_reportsAll(t *testing.T) {
	t.Parallel()

	err := checkMissingIDs(t.Context(), &stubBilling{})
	if err == nil {
		t.Fatal("checkMissingIDs(zero stub) = nil, want error")
	}

	for _, want := range []string{"empty customer", "empty plan", "CancelSubscription(empty)", "GetInvoice(empty)"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("checkMissingIDs() = %v, want containing %q", err, want)
		}
	}
}

func TestCheckMissingIDs_customerError(t *testing.T) {
	t.Parallel()

	boom := errors.New("billingtest: boom")
	stub := &stubBilling{createCustomerErr: boom}

	if err := checkMissingIDs(t.Context(), stub); !errors.Is(err, boom) {
		t.Fatalf("checkMissingIDs() = %v, want wrap of boom", err)
	}
}

func TestCheckMissingIDs_sentinelsPass(t *testing.T) {
	t.Parallel()

	// A stub enforcing the missing-ID contract passes every branch.
	enforcing := &missingIDStubBilling{stub: healthyStubBilling()}

	mustPass(t, checkMissingIDs(t.Context(), enforcing))
}

// missingIDStubBilling enforces the missing-ID sentinel contract.
type missingIDStubBilling struct {
	stub *stubBilling
}

func (m *missingIDStubBilling) CreateCustomer(ctx context.Context, name, email, key string) (billing.Customer, error) {
	return m.stub.CreateCustomer(ctx, name, email, key)
}

func (m *missingIDStubBilling) CreateSubscription(_ context.Context, customerID, planID, _ string) (billing.Subscription, error) {
	if customerID == "" {
		return billing.Subscription{}, billing.ErrMissingCustomerID
	}

	if planID == "" {
		return billing.Subscription{}, billing.ErrMissingPlanID
	}

	return m.stub.subscription, nil
}

func (m *missingIDStubBilling) CancelSubscription(_ context.Context, id string) error {
	if id == "" {
		return billing.ErrMissingSubscriptionID
	}

	return nil
}

func (m *missingIDStubBilling) GetInvoice(_ context.Context, customerID string) (billing.Invoice, error) {
	if customerID == "" {
		return billing.Invoice{}, billing.ErrMissingCustomerID
	}

	return m.stub.invoice, nil
}

func (m *missingIDStubBilling) Close() error { return m.stub.Close() }

func TestCheckClose_success(t *testing.T) {
	t.Parallel()

	mustPass(t, checkClose(healthyStubBilling()))
}

func TestCheckClose_failures(t *testing.T) {
	t.Parallel()

	boom := errors.New("billingtest: boom")

	t.Run("first close error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubBilling()
		stub.closeErr = boom

		if err := checkClose(stub); !errors.Is(err, boom) {
			t.Fatalf("checkClose() = %v, want wrap of boom", err)
		}
	})

	t.Run("second close error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubBilling()
		stub.closeAgainErr = boom

		if err := checkClose(stub); err == nil {
			t.Fatal("checkClose() = nil, want second-close error")
		} else if !strings.Contains(err.Error(), "Close() second") {
			t.Fatalf("checkClose() = %v, want second-close error", err)
		}
	})
}

func TestCheck_concurrent(t *testing.T) {
	t.Parallel()

	var wg sync.WaitGroup

	for i := 0; i < 8; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			ctx := context.Background()
			stub := healthyStubBilling()

			if err := checkLifecycle(ctx, stub); err != nil {
				t.Errorf("checkLifecycle() = %v, want nil", err)
			}

			if err := checkCancel(ctx, healthyStubBilling()); err != nil {
				t.Errorf("checkCancel() = %v, want nil", err)
			}

			if err := checkClose(healthyStubBilling()); err != nil {
				t.Errorf("checkClose() = %v, want nil", err)
			}
		}()
	}

	wg.Wait()
}
