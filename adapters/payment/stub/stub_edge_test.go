package stub

import (
	"errors"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/core/payment"
)

func TestCreatePayment_distinctIDsConcurrent(t *testing.T) {
	t.Parallel()

	p := newStub(t, true)
	const workers = 64

	var wg sync.WaitGroup
	ids := make([]string, workers)
	errs := make([]error, workers)
	for w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := p.CreatePayment(t.Context(), payment.Request{Amount: 100, Currency: "USD"})
			ids[w] = res.ID
			errs[w] = err
		}()
	}
	wg.Wait()

	seen := make(map[string]struct{}, workers)
	for w, err := range errs {
		if err != nil {
			t.Fatalf("worker %d CreatePayment() = %v", w, err)
		}
		if ids[w] == "" {
			t.Fatalf("worker %d empty ID", w)
		}
		if _, dup := seen[ids[w]]; dup {
			t.Fatalf("duplicate payment ID %q", ids[w])
		}
		seen[ids[w]] = struct{}{}
	}
}

func TestRefund_keyIgnored(t *testing.T) {
	t.Parallel()

	p := newStub(t, true)
	res, err := p.CreatePayment(t.Context(), payment.Request{Amount: 100, Currency: "USD"})
	if err != nil {
		t.Fatalf("CreatePayment() = %v", err)
	}
	if err := p.Refund(t.Context(), res.ID, 100, "arbitrary-key"); err != nil {
		t.Errorf("Refund(key) = %v, want nil (stub ignores key)", err)
	}
}

func TestCreatePayment_nilContext(t *testing.T) {
	t.Parallel()

	p := newStub(t, true)
	if _, err := p.CreatePayment(nil, payment.Request{Amount: 10, Currency: "USD"}); err != nil { //nolint:staticcheck // deliberately exercises nil-context handling
		t.Errorf("CreatePayment(nil ctx) = %v, want nil", err)
	}
}

func TestWebhookEvent_emptyPayload(t *testing.T) {
	t.Parallel()

	p := newStub(t, true)
	if _, err := p.WebhookEvent(t.Context(), nil, ""); !errors.Is(err, payment.ErrInvalidSignature) {
		t.Errorf("WebhookEvent(nil, empty) = %v, want ErrInvalidSignature", err)
	}
}

func TestClose_idempotent(t *testing.T) {
	t.Parallel()

	p := newStub(t, true)
	if err := p.Close(); err != nil {
		t.Fatalf("Close() = %v, want nil", err)
	}
	if err := p.Close(); err != nil {
		t.Errorf("second Close() = %v, want nil", err)
	}
}
