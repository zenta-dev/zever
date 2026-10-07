// Package billingtest provides the conformance kit third-party billing adapters run to prove backend parity.
package billingtest

import (
	"context"
	"errors"
	"fmt"
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

	if err := checkOpenRegister(); err != nil {
		t.Fatal(err)
	}
}

// checkOpenRegister proves the open/register round-trip against the
// shared registry. It returns a descriptive error on the first
// contract violation so unit tests can drive every branch.
func checkOpenRegister() error {
	if _, err := billing.Open(billing.Adapter("conformance-missing-adapter"), billing.Options{}); !errors.Is(err, billing.ErrUnknownAdapter) {
		return fmt.Errorf("billingtest: Open(missing) err = %w, want ErrUnknownAdapter", err)
	}

	probe := billing.Adapter("conformance-probe-billing")

	if err := billing.Register(probe, nil); !errors.Is(err, billing.ErrNilFactory) {
		return fmt.Errorf("billingtest: Register(nil) err = %w, want ErrNilFactory", err)
	}

	stub := func(billing.Options) (billing.Billing, error) {
		return nil, errors.New("billingtest: probe factory must not run")
	}

	_ = billing.Register(probe, stub)

	if err := billing.Register(probe, stub); !errors.Is(err, billing.ErrDuplicate) {
		return fmt.Errorf("billingtest: Register(duplicate) err = %w, want ErrDuplicate", err)
	}

	return nil
}

func conformanceLifecycle(t *testing.T, factory func(t *testing.T) billing.Billing) {
	t.Helper()

	if err := checkLifecycle(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkLifecycle proves the customer-subscription-invoice linkage. Soft
// linkage mismatches join into one error so every violation is
// reported; hard failures abort at the first one.
func checkLifecycle(ctx context.Context, b billing.Billing) error {
	cust, err := b.CreateCustomer(ctx, "Kit User", "kit@example.com", "kit-key-01")
	if err != nil {
		return fmt.Errorf("billingtest: CreateCustomer() error = %w", err)
	}

	if cust.ID == "" {
		return errors.New("billingtest: CreateCustomer() ID is empty")
	}

	sub, err := b.CreateSubscription(ctx, cust.ID, "plan-kit", "kit-key-02")
	if err != nil {
		return fmt.Errorf("billingtest: CreateSubscription() error = %w", err)
	}

	if sub.ID == "" {
		return errors.New("billingtest: CreateSubscription() ID is empty")
	}

	var errs []error

	if sub.CustomerID != cust.ID {
		errs = append(errs, fmt.Errorf("billingtest: Subscription.CustomerID = %q, want %q", sub.CustomerID, cust.ID))
	}

	if sub.PlanID != "plan-kit" {
		errs = append(errs, fmt.Errorf("billingtest: Subscription.PlanID = %q, want plan-kit", sub.PlanID))
	}

	inv, err := b.GetInvoice(ctx, cust.ID)
	if err != nil {
		errs = append(errs, fmt.Errorf("billingtest: GetInvoice() error = %w", err))
	} else if inv.ID == "" {
		errs = append(errs, errors.New("billingtest: GetInvoice() ID is empty"))
	}

	return errors.Join(errs...)
}

func conformanceCancel(t *testing.T, factory func(t *testing.T) billing.Billing) {
	t.Helper()

	if err := checkCancel(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkCancel proves cancel semantics, tolerating a second cancel
// that reports ErrNotFound for backends that drop canceled records.
func checkCancel(ctx context.Context, b billing.Billing) error {
	cust, err := b.CreateCustomer(ctx, "Cancel User", "cancel@example.com", "")
	if err != nil {
		return fmt.Errorf("billingtest: CreateCustomer() error = %w", err)
	}

	sub, err := b.CreateSubscription(ctx, cust.ID, "plan-kit", "")
	if err != nil {
		return fmt.Errorf("billingtest: CreateSubscription() error = %w", err)
	}

	if err := b.CancelSubscription(ctx, sub.ID); err != nil {
		return fmt.Errorf("billingtest: CancelSubscription() error = %w", err)
	}

	if err := b.CancelSubscription(ctx, sub.ID); err != nil && !errors.Is(err, billing.ErrNotFound) {
		return fmt.Errorf("billingtest: CancelSubscription(again) err = %w, want nil or ErrNotFound", err)
	}

	return nil
}

func conformanceNotFound(t *testing.T, factory func(t *testing.T) billing.Billing) {
	t.Helper()

	if err := checkNotFound(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkNotFound proves unknown IDs surface ErrNotFound on every method.
func checkNotFound(ctx context.Context, b billing.Billing) error {
	var errs []error

	if _, err := b.CreateSubscription(ctx, "cus_missing", "plan-kit", ""); !errors.Is(err, billing.ErrNotFound) {
		errs = append(errs, fmt.Errorf("billingtest: CreateSubscription(unknown customer) err = %w, want ErrNotFound", err))
	}

	if err := b.CancelSubscription(ctx, "sub_missing"); !errors.Is(err, billing.ErrNotFound) {
		errs = append(errs, fmt.Errorf("billingtest: CancelSubscription(missing) err = %w, want ErrNotFound", err))
	}

	if _, err := b.GetInvoice(ctx, "cus_missing"); !errors.Is(err, billing.ErrNotFound) {
		errs = append(errs, fmt.Errorf("billingtest: GetInvoice(missing) err = %w, want ErrNotFound", err))
	}

	return errors.Join(errs...)
}

func conformanceMissingIDs(t *testing.T, factory func(t *testing.T) billing.Billing) {
	t.Helper()

	if err := checkMissingIDs(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkMissingIDs proves empty IDs surface the missing-ID sentinels.
func checkMissingIDs(ctx context.Context, b billing.Billing) error {
	var errs []error

	if _, err := b.CreateSubscription(ctx, "", "plan-kit", ""); !errors.Is(err, billing.ErrMissingCustomerID) {
		errs = append(errs, fmt.Errorf("billingtest: CreateSubscription(empty customer) err = %w, want ErrMissingCustomerID", err))
	}

	cust, err := b.CreateCustomer(ctx, "Kit User", "kit@example.com", "")
	if err != nil {
		return fmt.Errorf("billingtest: CreateCustomer() error = %w", err)
	}

	if _, err := b.CreateSubscription(ctx, cust.ID, "", ""); !errors.Is(err, billing.ErrMissingPlanID) {
		errs = append(errs, fmt.Errorf("billingtest: CreateSubscription(empty plan) err = %w, want ErrMissingPlanID", err))
	}

	if err := b.CancelSubscription(ctx, ""); !errors.Is(err, billing.ErrMissingSubscriptionID) {
		errs = append(errs, fmt.Errorf("billingtest: CancelSubscription(empty) err = %w, want ErrMissingSubscriptionID", err))
	}

	if _, err := b.GetInvoice(ctx, ""); !errors.Is(err, billing.ErrMissingCustomerID) {
		errs = append(errs, fmt.Errorf("billingtest: GetInvoice(empty) err = %w, want ErrMissingCustomerID", err))
	}

	return errors.Join(errs...)
}

func conformanceClose(t *testing.T, factory func(t *testing.T) billing.Billing) {
	t.Helper()

	if err := checkClose(factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkClose proves Close is idempotent.
func checkClose(b billing.Billing) error {
	if err := b.Close(); err != nil {
		return fmt.Errorf("billingtest: Close() error = %w", err)
	}

	if err := b.Close(); err != nil {
		return fmt.Errorf("billingtest: Close() second error = %w, want nil", err)
	}

	return nil
}
