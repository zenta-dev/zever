// Package billingtest provides the conformance kit third-party billing adapters run to prove backend parity.
package billingtest

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/billing"
)

// Conformance verifies factory-built billings implement the
// billing.Billing contract: open/register round-trip, customer then
// subscription then invoice lifecycle, cancel semantics, missing-ID
// and not-found sentinels, and Close. Each subtest takes a fresh
// instance from factory so cases stay isolated. Tests never call
// time.Sleep and never touch the network (live-provider adapters
// prove against recorded fakes in their own packages; the kit runs
// against the stub or sandbox wiring).
//
// Documented stub exemption: the stub ignores idempotency keys and
// mints zero-amount paid invoices; the kit never asserts dedup
// behavior or invoice amounts, only lifecycle linkage (invoice
// exists after subscription) and error sentinels.
func Conformance(t *testing.T, factory func(t *testing.T) billing.Billing) {
	t.Helper()

	t.Run("OpenRegister", func(t *testing.T) { conformanceOpenRegister(t) })
	t.Run("CustomerSubscriptionInvoice", func(t *testing.T) { conformanceLifecycle(t, factory) })
	t.Run("Cancel", func(t *testing.T) { conformanceCancel(t, factory) })
	t.Run("NotFound", func(t *testing.T) { conformanceNotFound(t, factory) })
	t.Run("MissingIDs", func(t *testing.T) { conformanceMissingIDs(t, factory) })
	t.Run("Close", func(t *testing.T) { conformanceClose(t, factory) })
}

func conformanceOpenRegister(t *testing.T) {
	t.Helper()

	if _, err := billing.Open(billing.Adapter("conformance-missing-adapter"), billing.Options{}); !errors.Is(err, billing.ErrUnknownAdapter) {
		t.Fatalf("Open(missing) err = %v, want ErrUnknownAdapter", err)
	}

	probe := billing.Adapter("conformance-probe-billing")

	if err := billing.Register(probe, nil); !errors.Is(err, billing.ErrNilFactory) {
		t.Fatalf("Register(nil) err = %v, want ErrNilFactory", err)
	}

	stub := func(billing.Options) (billing.Billing, error) {
		return nil, errors.New("billingtest: probe factory must not run")
	}

	_ = billing.Register(probe, stub)

	if err := billing.Register(probe, stub); !errors.Is(err, billing.ErrDuplicate) {
		t.Fatalf("Register(duplicate) err = %v, want ErrDuplicate", err)
	}
}

func conformanceLifecycle(t *testing.T, factory func(t *testing.T) billing.Billing) {
	t.Helper()

	ctx := t.Context()
	b := factory(t)

	cust, err := b.CreateCustomer(ctx, "Kit User", "kit@example.com", "kit-key-01")
	if err != nil {
		t.Fatalf("CreateCustomer() error = %v", err)
	}

	if cust.ID == "" {
		t.Fatal("CreateCustomer() ID is empty")
	}

	sub, err := b.CreateSubscription(ctx, cust.ID, "plan-kit", "kit-key-02")
	if err != nil {
		t.Fatalf("CreateSubscription() error = %v", err)
	}

	if sub.ID == "" {
		t.Fatal("CreateSubscription() ID is empty")
	}

	if sub.CustomerID != cust.ID {
		t.Errorf("Subscription.CustomerID = %q, want %q", sub.CustomerID, cust.ID)
	}

	if sub.PlanID != "plan-kit" {
		t.Errorf("Subscription.PlanID = %q, want plan-kit", sub.PlanID)
	}

	inv, err := b.GetInvoice(ctx, cust.ID)
	if err != nil {
		t.Fatalf("GetInvoice() error = %v", err)
	}

	if inv.ID == "" {
		t.Error("GetInvoice() ID is empty")
	}
}

func conformanceCancel(t *testing.T, factory func(t *testing.T) billing.Billing) {
	t.Helper()

	ctx := t.Context()
	b := factory(t)

	cust, err := b.CreateCustomer(ctx, "Cancel User", "cancel@example.com", "")
	if err != nil {
		t.Fatalf("CreateCustomer() error = %v", err)
	}

	sub, err := b.CreateSubscription(ctx, cust.ID, "plan-kit", "")
	if err != nil {
		t.Fatalf("CreateSubscription() error = %v", err)
	}

	if err := b.CancelSubscription(ctx, sub.ID); err != nil {
		t.Fatalf("CancelSubscription() error = %v", err)
	}

	if err := b.CancelSubscription(ctx, sub.ID); err != nil && !errors.Is(err, billing.ErrNotFound) {
		t.Errorf("CancelSubscription(again) err = %v, want nil or ErrNotFound", err)
	}
}

func conformanceNotFound(t *testing.T, factory func(t *testing.T) billing.Billing) {
	t.Helper()

	ctx := t.Context()
	b := factory(t)

	if _, err := b.CreateSubscription(ctx, "cus_missing", "plan-kit", ""); !errors.Is(err, billing.ErrNotFound) {
		t.Errorf("CreateSubscription(unknown customer) err = %v, want ErrNotFound", err)
	}

	if err := b.CancelSubscription(ctx, "sub_missing"); !errors.Is(err, billing.ErrNotFound) {
		t.Errorf("CancelSubscription(missing) err = %v, want ErrNotFound", err)
	}

	if _, err := b.GetInvoice(ctx, "cus_missing"); !errors.Is(err, billing.ErrNotFound) {
		t.Errorf("GetInvoice(missing) err = %v, want ErrNotFound", err)
	}
}

func conformanceMissingIDs(t *testing.T, factory func(t *testing.T) billing.Billing) {
	t.Helper()

	ctx := t.Context()
	b := factory(t)

	if _, err := b.CreateSubscription(ctx, "", "plan-kit", ""); !errors.Is(err, billing.ErrMissingCustomerID) {
		t.Errorf("CreateSubscription(empty customer) err = %v, want ErrMissingCustomerID", err)
	}

	cust, err := b.CreateCustomer(ctx, "Kit User", "kit@example.com", "")
	if err != nil {
		t.Fatalf("CreateCustomer() error = %v", err)
	}

	if _, err := b.CreateSubscription(ctx, cust.ID, "", ""); !errors.Is(err, billing.ErrMissingPlanID) {
		t.Errorf("CreateSubscription(empty plan) err = %v, want ErrMissingPlanID", err)
	}

	if err := b.CancelSubscription(ctx, ""); !errors.Is(err, billing.ErrMissingSubscriptionID) {
		t.Errorf("CancelSubscription(empty) err = %v, want ErrMissingSubscriptionID", err)
	}

	if _, err := b.GetInvoice(ctx, ""); !errors.Is(err, billing.ErrMissingCustomerID) {
		t.Errorf("GetInvoice(empty) err = %v, want ErrMissingCustomerID", err)
	}
}

func conformanceClose(t *testing.T, factory func(t *testing.T) billing.Billing) {
	t.Helper()

	b := factory(t)

	if err := b.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := b.Close(); err != nil {
		t.Errorf("Close() second error = %v, want nil", err)
	}
}
