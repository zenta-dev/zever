package stub

import (
	"errors"
	"sync"
	"testing"

	"github.com/oklog/ulid/v2"

	"github.com/zenta-dev/zever/billing"
)

type errReader struct{}

func (errReader) Read(_ []byte) (int, error) {
	return 0, errors.New("boom")
}

func TestRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	b := New()

	c, err := b.CreateCustomer(ctx, "Ada", "ada@example.com", "")
	if err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}

	s, err := b.CreateSubscription(ctx, c.ID, "plan_pro", "")
	if err != nil {
		t.Fatalf("CreateSubscription: %v", err)
	}

	if s.Status != billing.SubscriptionActive {
		t.Fatalf("Status = %q, want %q", s.Status, billing.SubscriptionActive)
	}

	inv, err := b.GetInvoice(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetInvoice: %v", err)
	}

	if inv.Status != billing.InvoicePaid {
		t.Fatalf("Invoice Status = %q, want %q", inv.Status, billing.InvoicePaid)
	}

	if inv.AmountDue != 0 {
		t.Fatalf("AmountDue = %d, want 0", inv.AmountDue)
	}

	if inv.Currency != "usd" {
		t.Fatalf("Currency = %q, want usd", inv.Currency)
	}

	if err := b.CancelSubscription(ctx, s.ID); err != nil {
		t.Fatalf("CancelSubscription: %v", err)
	}

	d, ok := b.(*driver)
	if !ok {
		t.Fatal("expected *driver")
	}

	d.mu.Lock()
	got := d.subscriptions[s.ID]
	d.mu.Unlock()

	if got.Status != billing.SubscriptionCanceled {
		t.Fatalf("after cancel Status = %q, want %q", got.Status, billing.SubscriptionCanceled)
	}
}

func TestGetInvoiceNotFound(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	b := New()

	c, err := b.CreateCustomer(ctx, "Bob", "bob@example.com", "")
	if err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}

	_, err = b.GetInvoice(ctx, c.ID)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var nf *billing.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("expected NotFoundError, got %T", err)
	}

	if nf.Resource != "invoice" {
		t.Fatalf("Resource = %q, want invoice", nf.Resource)
	}

	if nf.ID != c.ID {
		t.Fatalf("ID = %q, want %q", nf.ID, c.ID)
	}

	if !errors.Is(err, billing.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestCreateSubscriptionCustomerMiss(t *testing.T) {
	t.Parallel()

	_, err := New().CreateSubscription(t.Context(), "cus_missing", "plan_pro", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var nf *billing.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("expected NotFoundError, got %T", err)
	}

	if nf.Resource != "customer" {
		t.Fatalf("Resource = %q, want customer", nf.Resource)
	}

	if nf.ID != "cus_missing" {
		t.Fatalf("ID = %q, want cus_missing", nf.ID)
	}

	if !errors.Is(err, billing.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestCancelSubscriptionNotFound(t *testing.T) {
	t.Parallel()

	err := New().CancelSubscription(t.Context(), "sub_missing")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var nf *billing.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("expected NotFoundError, got %T", err)
	}

	if nf.Resource != "subscription" {
		t.Fatalf("Resource = %q, want subscription", nf.Resource)
	}

	if !errors.Is(err, billing.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestEmptyIDGuards(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	b := New()

	if _, err := b.CreateSubscription(ctx, "", "plan_pro", ""); !errors.Is(err, billing.ErrMissingCustomerID) {
		t.Fatalf("empty customerID: expected ErrMissingCustomerID, got %v", err)
	}

	c, err := b.CreateCustomer(ctx, "Eve", "eve@example.com", "")
	if err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}

	if _, err := b.CreateSubscription(ctx, c.ID, "", ""); !errors.Is(err, billing.ErrMissingPlanID) {
		t.Fatalf("empty planID: expected ErrMissingPlanID, got %v", err)
	}

	if err := b.CancelSubscription(ctx, ""); !errors.Is(err, billing.ErrMissingSubscriptionID) {
		t.Fatalf("empty subscription id: expected ErrMissingSubscriptionID, got %v", err)
	}

	if _, err := b.GetInvoice(ctx, ""); !errors.Is(err, billing.ErrMissingCustomerID) {
		t.Fatalf("empty customerID invoice: expected ErrMissingCustomerID, got %v", err)
	}
}

func TestOpen(t *testing.T) {
	t.Parallel()

	b, err := Open(billing.Options{})
	if err != nil {
		t.Fatalf("Open zero: %v", err)
	}

	if cerr := b.Close(); cerr != nil {
		t.Fatalf("Close: %v", cerr)
	}

	_, oerr := Open(billing.Options{Endpoint: "not-a-url"})
	if !errors.Is(oerr, billing.ErrInvalidOptions) {
		t.Fatalf("invalid options: expected ErrInvalidOptions, got %v", oerr)
	}
}

func TestClose(t *testing.T) {
	t.Parallel()

	if err := New().Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestNewULIDFallback(t *testing.T) {
	t.Parallel()

	got := newULIDFromReader(errReader{})
	if _, err := ulid.Parse(got); err != nil {
		t.Fatalf("fallback ULID %q does not parse: %v", got, err)
	}
}

func TestIDUniqueness(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	b := New()
	seen := make(map[string]struct{}, 100)

	for range 100 {
		c, err := b.CreateCustomer(ctx, "N", "n@example.com", "")
		if err != nil {
			t.Fatalf("CreateCustomer: %v", err)
		}

		seen[c.ID] = struct{}{}
	}

	if len(seen) != 100 {
		t.Fatalf("distinct IDs = %d, want 100", len(seen))
	}
}

func TestConcurrent(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	b := New()

	var wg sync.WaitGroup

	for range 50 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			c, err := b.CreateCustomer(ctx, "C", "c@example.com", "")
			if err != nil {
				t.Errorf("CreateCustomer: %v", err)
				return
			}

			s, err := b.CreateSubscription(ctx, c.ID, "plan_pro", "")
			if err != nil {
				t.Errorf("CreateSubscription: %v", err)
				return
			}

			if _, err := b.GetInvoice(ctx, c.ID); err != nil {
				t.Errorf("GetInvoice: %v", err)
				return
			}

			if err := b.CancelSubscription(ctx, s.ID); err != nil {
				t.Errorf("CancelSubscription: %v", err)
			}
		}()
	}

	wg.Wait()
}
