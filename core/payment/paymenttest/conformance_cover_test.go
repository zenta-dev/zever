package paymenttest

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/core/payment"
)

// stubPayment is a scriptable payment.Payment double implementing the
// documented contract by default. Override fields drive each
// conformance failure branch; refund hooks replace per-condition
// validation when non-nil.
type stubPayment struct {
	mu            sync.Mutex
	id            string
	amount        int64
	currency      string
	refunded      int64
	created       bool
	gets          int
	closes        int
	createErr     error
	emptyID       bool
	wrongEcho     bool
	getErr        error
	getFullErr    error
	wrongGetID    bool
	missingErr    error
	invalidBypass bool
	refundPartial error
	refundRest    error
	statusPartial payment.PaymentStatus
	statusFull    payment.PaymentStatus
	refundZero    func() error
	refundOver    func() error
	refundMissing func() error
	refundEmpty   func() error
	webhookErr    error
	closeErr      error
	closeAgainErr error
}

func healthyStubPayment() *stubPayment {
	return &stubPayment{
		missingErr: payment.ErrNotFound,
		webhookErr: payment.ErrInvalidSignature,
	}
}

func (s *stubPayment) CreatePayment(_ context.Context, req payment.Request) (payment.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.createErr != nil {
		return payment.Result{}, s.createErr
	}

	if !s.invalidBypass {
		if req.Amount <= 0 {
			return payment.Result{}, payment.ErrInvalidAmount
		}

		if req.Currency == "" {
			return payment.Result{}, payment.ErrMissingCurrency
		}

		if req.Method != "" && req.Method != payment.MethodCard && req.Method != payment.MethodBankTransfer {
			return payment.Result{}, payment.ErrUnsupportedMethod
		}
	}

	s.created = true
	s.id = "pay_kit"
	s.amount = req.Amount
	s.currency = req.Currency
	s.refunded = 0

	res := payment.Result{ID: s.id, Status: payment.PaymentSucceeded, Amount: req.Amount, Currency: req.Currency}
	if s.emptyID {
		res.ID = ""
	}

	if s.wrongEcho {
		res.Amount++
	}

	return res, nil
}

func (s *stubPayment) Refund(_ context.Context, id string, amount int64, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.refundPartial != nil && s.refunded == 0 && amount == 400 {
		return s.refundPartial
	}

	if id == "" {
		if s.refundEmpty != nil {
			return s.refundEmpty()
		}

		return payment.ErrMissingPaymentID
	}

	if !s.created || id != s.id {
		if s.refundMissing != nil {
			return s.refundMissing()
		}

		return payment.ErrNotFound
	}

	if amount <= 0 {
		if s.refundZero != nil {
			return s.refundZero()
		}

		return payment.ErrInvalidAmount
	}

	if s.refunded+amount > s.amount {
		if s.refundOver != nil {
			return s.refundOver()
		}

		return payment.ErrAmountMismatch
	}

	if s.refundRest != nil && s.refunded > 0 {
		return s.refundRest
	}

	s.refunded += amount

	return nil
}

func (s *stubPayment) GetPayment(_ context.Context, id string) (payment.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.gets++

	if s.gets > 1 && s.getFullErr != nil {
		return payment.Result{}, s.getFullErr
	}

	if s.gets == 1 && s.getErr != nil {
		return payment.Result{}, s.getErr
	}

	if !s.created || id != s.id {
		if s.missingErr != nil {
			return payment.Result{}, s.missingErr
		}

		return payment.Result{}, nil
	}

	status := payment.PaymentSucceeded

	switch {
	case s.refunded >= s.amount && s.amount > 0:
		status = payment.PaymentRefunded
		if s.statusFull != "" {
			status = s.statusFull
		}
	case s.refunded > 0:
		status = payment.PaymentPartiallyRefunded
		if s.statusPartial != "" {
			status = s.statusPartial
		}
	}

	res := payment.Result{ID: s.id, Status: status, Amount: s.amount, Currency: s.currency}
	if s.wrongGetID {
		res.ID = "pay_other"
	}

	return res, nil
}

func (s *stubPayment) WebhookEvent(_ context.Context, _ []byte, _ string) (payment.Event, error) {
	if s.webhookErr != nil {
		return payment.Event{}, s.webhookErr
	}

	return payment.Event{Type: "kit.event"}, nil
}

func (s *stubPayment) Close() error {
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

func boom() error { return errors.New("paymenttest: boom") }

func TestCheckOpenRegister_success(t *testing.T) {
	t.Parallel()

	mustPass(t, checkOpenRegister())
}

func TestCheckCreateGet_success(t *testing.T) {
	t.Parallel()

	mustPass(t, checkCreateGet(t.Context(), healthyStubPayment()))
}

func TestCheckCreateGet_failures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		mutate   func(*stubPayment)
		contains string
	}{
		{"create error", func(s *stubPayment) { s.createErr = boom() }, "CreatePayment() error"},
		{"empty id", func(s *stubPayment) { s.emptyID = true }, "CreatePayment() ID is empty"},
		{"echo mismatch", func(s *stubPayment) { s.wrongEcho = true }, "want amount 2500 usd echoed"},
		{"get error", func(s *stubPayment) { s.getErr = boom() }, "GetPayment() error"},
		{"get id mismatch", func(s *stubPayment) { s.wrongGetID = true }, "GetPayment() ID"},
		{"missing not notfound", func(s *stubPayment) { s.missingErr = nil }, "GetPayment(missing)"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			stub := healthyStubPayment()
			tc.mutate(stub)

			err := checkCreateGet(t.Context(), stub)
			if err == nil {
				t.Fatalf("checkCreateGet() = nil, want error containing %q", tc.contains)
			}
			if !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("checkCreateGet() = %v, want containing %q", err, tc.contains)
			}
		})
	}
}

func TestCheckCreateGet_createErrorWraps(t *testing.T) {
	t.Parallel()

	sentinel := boom()
	stub := healthyStubPayment()
	stub.createErr = sentinel

	if err := checkCreateGet(t.Context(), stub); !errors.Is(err, sentinel) {
		t.Fatalf("checkCreateGet() = %v, want wrap of sentinel", err)
	}
}

func TestCheckInvalidRequest_success(t *testing.T) {
	t.Parallel()

	mustPass(t, checkInvalidRequest(t.Context(), healthyStubPayment()))
}

func TestCheckInvalidRequest_reportsAll(t *testing.T) {
	t.Parallel()

	stub := healthyStubPayment()
	stub.invalidBypass = true

	err := checkInvalidRequest(t.Context(), stub)
	if err == nil {
		t.Fatal("checkInvalidRequest(bypass stub) = nil, want joined errors")
	}

	for _, want := range []string{"CreatePayment(zero)", "CreatePayment(negative)", "CreatePayment(no currency)", "CreatePayment(bad method)"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("checkInvalidRequest() = %v, want containing %q", err, want)
		}
	}
}

func TestCheckRefund_success(t *testing.T) {
	t.Parallel()

	mustPass(t, checkRefund(t.Context(), healthyStubPayment()))
}

func TestCheckRefund_failures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		mutate   func(*stubPayment)
		contains string
	}{
		{"create error", func(s *stubPayment) { s.createErr = boom() }, "CreatePayment() error"},
		{"partial error", func(s *stubPayment) { s.refundPartial = boom() }, "Refund(partial) error"},
		{"get error", func(s *stubPayment) { s.getErr = boom() }, "GetPayment() error"},
		{"get full error", func(s *stubPayment) { s.getFullErr = boom() }, "GetPayment() error"},
		{"partial status wrong", func(s *stubPayment) { s.statusPartial = payment.PaymentSucceeded }, "want partially_refunded"},
		{"rest error", func(s *stubPayment) { s.refundRest = boom() }, "Refund(rest) error"},
		{"full status wrong", func(s *stubPayment) { s.statusFull = payment.PaymentPartiallyRefunded }, "want refunded after full refund"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			stub := healthyStubPayment()
			tc.mutate(stub)

			err := checkRefund(t.Context(), stub)
			if err == nil {
				t.Fatalf("checkRefund() = nil, want error containing %q", tc.contains)
			}
			if !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("checkRefund() = %v, want containing %q", err, tc.contains)
			}
		})
	}
}

func TestCheckRefundMismatch_success(t *testing.T) {
	t.Parallel()

	mustPass(t, checkRefundMismatch(t.Context(), healthyStubPayment()))
}

func TestCheckRefundMismatch_reportsAll(t *testing.T) {
	t.Parallel()

	stub := healthyStubPayment()
	stub.refundZero = boom
	stub.refundOver = boom
	stub.refundMissing = boom
	stub.refundEmpty = boom

	err := checkRefundMismatch(t.Context(), stub)
	if err == nil {
		t.Fatal("checkRefundMismatch(broken stub) = nil, want joined errors")
	}

	for _, want := range []string{"Refund(zero)", "Refund(over total)", "Refund(missing)", "Refund(empty id)"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("checkRefundMismatch() = %v, want containing %q", err, want)
		}
	}
}

func TestCheckRefundMismatch_emptyIDNil(t *testing.T) {
	t.Parallel()

	stub := healthyStubPayment()
	stub.refundEmpty = func() error { return nil }

	err := checkRefundMismatch(t.Context(), stub)
	if err == nil {
		t.Fatal("checkRefundMismatch() = nil, want empty-id error")
	}
	if !strings.Contains(err.Error(), "Refund(empty id) = nil") {
		t.Fatalf("checkRefundMismatch() = %v, want empty-id nil error", err)
	}
}

func TestCheckRefundMismatch_createErrorWraps(t *testing.T) {
	t.Parallel()

	sentinel := boom()
	stub := healthyStubPayment()
	stub.createErr = sentinel

	if err := checkRefundMismatch(t.Context(), stub); !errors.Is(err, sentinel) {
		t.Fatalf("checkRefundMismatch() = %v, want wrap of sentinel", err)
	}
}

func TestCheckRefundMismatch_emptyIDMissingSentinelPasses(t *testing.T) {
	t.Parallel()

	stub := healthyStubPayment()
	stub.refundEmpty = func() error { return payment.ErrNotFound }

	mustPass(t, checkRefundMismatch(t.Context(), stub))
}

func TestCheckWebhook_success(t *testing.T) {
	t.Parallel()

	mustPass(t, checkWebhook(t.Context(), healthyStubPayment()))
}

func TestCheckWebhook_failures(t *testing.T) {
	t.Parallel()

	t.Run("nil error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubPayment()
		stub.webhookErr = nil

		if err := checkWebhook(t.Context(), stub); err == nil {
			t.Fatal("checkWebhook() = nil, want ErrInvalidSignature")
		} else if !strings.Contains(err.Error(), "WebhookEvent(stub)") {
			t.Fatalf("checkWebhook() = %v, want webhook error", err)
		}
	})

	t.Run("wrong error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubPayment()
		stub.webhookErr = boom()

		if err := checkWebhook(t.Context(), stub); !strings.Contains(err.Error(), "want ErrInvalidSignature") {
			t.Fatalf("checkWebhook() = %v, want signature error", err)
		}
	})
}

func TestCheckClose_success(t *testing.T) {
	t.Parallel()

	mustPass(t, checkClose(healthyStubPayment()))
}

func TestCheckClose_failures(t *testing.T) {
	t.Parallel()

	t.Run("first close error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubPayment()
		stub.closeErr = boom()

		if err := checkClose(stub); !errors.Is(err, stub.closeErr) {
			t.Fatalf("checkClose() = %v, want wrap of close error", err)
		}
	})

	t.Run("second close error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubPayment()
		stub.closeAgainErr = boom()

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

			if err := checkCreateGet(ctx, healthyStubPayment()); err != nil {
				t.Errorf("checkCreateGet() = %v, want nil", err)
			}

			if err := checkRefund(ctx, healthyStubPayment()); err != nil {
				t.Errorf("checkRefund() = %v, want nil", err)
			}

			if err := checkClose(healthyStubPayment()); err != nil {
				t.Errorf("checkClose() = %v, want nil", err)
			}
		}()
	}

	wg.Wait()
}
