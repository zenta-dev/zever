package stub

import (
	"errors"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/payment"
)

func newStub(t *testing.T, autoApprove bool) payment.Payment {
	t.Helper()

	p, err := New(payment.Options{AutoApprove: autoApprove})
	if err != nil {
		t.Fatal(err)
	}

	return p
}

func TestCreateGetRefund_roundtrip(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	p := newStub(t, true)

	res, err := p.CreatePayment(ctx, payment.Request{Amount: 100, Currency: "USD", Method: payment.MethodCard})
	if err != nil {
		t.Fatalf("CreatePayment err = %v", err)
	}

	if res.ID == "" {
		t.Fatal("CreatePayment ID empty")
	}

	if res.Status != payment.PaymentSucceeded {
		t.Fatalf("CreatePayment status = %v, want %v", res.Status, payment.PaymentSucceeded)
	}

	got, err := p.GetPayment(ctx, res.ID)
	if err != nil {
		t.Fatalf("GetPayment err = %v", err)
	}

	if got != res {
		t.Fatalf("GetPayment = %+v, want %+v", got, res)
	}

	if err = p.Refund(ctx, res.ID, 100, ""); err != nil {
		t.Fatalf("Refund err = %v", err)
	}

	got, err = p.GetPayment(ctx, res.ID)
	if err != nil {
		t.Fatalf("GetPayment after refund err = %v", err)
	}

	if got.Status != payment.PaymentRefunded {
		t.Fatalf("status after full refund = %v, want %v", got.Status, payment.PaymentRefunded)
	}

	if err := p.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}
}

func TestRefund_partialThenFull_statuses(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	p := newStub(t, true)

	res, err := p.CreatePayment(ctx, payment.Request{Amount: 100, Currency: "USD"})
	if err != nil {
		t.Fatalf("CreatePayment err = %v", err)
	}

	if err = p.Refund(ctx, res.ID, 40, ""); err != nil {
		t.Fatalf("partial Refund err = %v", err)
	}

	got, err := p.GetPayment(ctx, res.ID)
	if err != nil {
		t.Fatalf("GetPayment err = %v", err)
	}

	if got.Status != payment.PaymentPartiallyRefunded {
		t.Fatalf("status after partial = %v, want %v", got.Status, payment.PaymentPartiallyRefunded)
	}

	if err = p.Refund(ctx, res.ID, 60, ""); err != nil {
		t.Fatalf("full Refund err = %v", err)
	}

	got, err = p.GetPayment(ctx, res.ID)
	if err != nil {
		t.Fatalf("GetPayment err = %v", err)
	}

	if got.Status != payment.PaymentRefunded {
		t.Fatalf("status after full = %v, want %v", got.Status, payment.PaymentRefunded)
	}
}

func TestCreate_pendingWhenAutoApproveFalse(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	p := newStub(t, false)

	res, err := p.CreatePayment(ctx, payment.Request{Amount: 50, Currency: "EUR", Method: payment.MethodBankTransfer})
	if err != nil {
		t.Fatalf("CreatePayment err = %v", err)
	}

	if res.Status != payment.PaymentPending {
		t.Fatalf("status = %v, want %v", res.Status, payment.PaymentPending)
	}
}

func TestGet_notFound(t *testing.T) {
	t.Parallel()

	p := newStub(t, true)

	_, err := p.GetPayment(t.Context(), "stub_999999")
	if !errors.Is(err, payment.ErrNotFound) {
		t.Fatalf("GetPayment err = %v, want ErrNotFound", err)
	}

	var nf *payment.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("err %T is not *NotFoundError", err)
	}

	if nf.PaymentID != "stub_999999" {
		t.Fatalf("PaymentID = %q, want %q", nf.PaymentID, "stub_999999")
	}
}

func TestRefund_notFound(t *testing.T) {
	t.Parallel()

	p := newStub(t, true)

	err := p.Refund(t.Context(), "stub_nope", 10, "")
	if !errors.Is(err, payment.ErrNotFound) {
		t.Fatalf("Refund err = %v, want ErrNotFound", err)
	}

	var nf *payment.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("err %T is not *NotFoundError", err)
	}

	if nf.PaymentID != "stub_nope" {
		t.Fatalf("PaymentID = %q, want %q", nf.PaymentID, "stub_nope")
	}
}

func TestRefund_overflowTotal(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	p := newStub(t, true)

	res, err := p.CreatePayment(ctx, payment.Request{Amount: 100, Currency: "USD"})
	if err != nil {
		t.Fatalf("CreatePayment err = %v", err)
	}

	err = p.Refund(ctx, res.ID, 150, "")
	if !errors.Is(err, payment.ErrAmountMismatch) {
		t.Fatalf("Refund err = %v, want ErrAmountMismatch", err)
	}

	var mm *payment.AmountMismatchError
	if !errors.As(err, &mm) {
		t.Fatalf("err %T is not *AmountMismatchError", err)
	}

	if mm.Expected != 100 || mm.Actual != 150 {
		t.Fatalf("mismatch = %+v, want Expected 100 Actual 150", mm)
	}
}

func TestRefund_overflowRemaining(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	p := newStub(t, true)

	res, err := p.CreatePayment(ctx, payment.Request{Amount: 100, Currency: "USD"})
	if err != nil {
		t.Fatalf("CreatePayment err = %v", err)
	}

	if err = p.Refund(ctx, res.ID, 60, ""); err != nil {
		t.Fatalf("first Refund err = %v", err)
	}

	err = p.Refund(ctx, res.ID, 50, "")
	if !errors.Is(err, payment.ErrAmountMismatch) {
		t.Fatalf("Refund err = %v, want ErrAmountMismatch", err)
	}

	var mm *payment.AmountMismatchError
	if !errors.As(err, &mm) {
		t.Fatalf("err %T is not *AmountMismatchError", err)
	}

	if mm.Expected != 40 || mm.Actual != 50 {
		t.Fatalf("mismatch = %+v, want Expected 40 Actual 50", mm)
	}
}

func TestCreate_invalid(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	p := newStub(t, true)

	if _, err := p.CreatePayment(ctx, payment.Request{Amount: 0, Currency: "USD"}); !errors.Is(err, payment.ErrInvalidAmount) {
		t.Fatalf("zero amount err = %v, want ErrInvalidAmount", err)
	}

	if _, err := p.CreatePayment(ctx, payment.Request{Amount: -5, Currency: "USD"}); !errors.Is(err, payment.ErrInvalidAmount) {
		t.Fatalf("negative amount err = %v, want ErrInvalidAmount", err)
	}

	if _, err := p.CreatePayment(ctx, payment.Request{Amount: 100, Currency: ""}); !errors.Is(err, payment.ErrMissingCurrency) {
		t.Fatalf("missing currency err = %v, want ErrMissingCurrency", err)
	}

	if _, err := p.CreatePayment(ctx, payment.Request{Amount: 100, Currency: "USD", Method: "crypto"}); !errors.Is(err, payment.ErrUnsupportedMethod) {
		t.Fatalf("bad method err = %v, want ErrUnsupportedMethod", err)
	}

	var um *payment.UnsupportedMethodError
	if _, err := p.CreatePayment(ctx, payment.Request{Amount: 100, Currency: "USD", Method: "crypto"}); !errors.As(err, &um) {
		t.Fatalf("err %T is not *UnsupportedMethodError", err)
	} else if um.Method != "crypto" {
		t.Fatalf("Method = %q, want %q", um.Method, "crypto")
	}
}

func TestRefund_invalidAmount(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	p := newStub(t, true)

	res, err := p.CreatePayment(ctx, payment.Request{Amount: 100, Currency: "USD"})
	if err != nil {
		t.Fatalf("CreatePayment err = %v", err)
	}

	if err := p.Refund(ctx, res.ID, 0, ""); !errors.Is(err, payment.ErrInvalidAmount) {
		t.Fatalf("zero refund err = %v, want ErrInvalidAmount", err)
	}

	if err := p.Refund(ctx, res.ID, -1, ""); !errors.Is(err, payment.ErrInvalidAmount) {
		t.Fatalf("negative refund err = %v, want ErrInvalidAmount", err)
	}
}

func TestNew_zeroOpts(t *testing.T) {
	t.Parallel()

	p, err := New(payment.Options{})
	if err != nil {
		t.Fatalf("New err = %v", err)
	}

	res, err := p.CreatePayment(t.Context(), payment.Request{Amount: 10, Currency: "USD"})
	if err != nil {
		t.Fatalf("CreatePayment err = %v", err)
	}

	if res.Status != payment.PaymentPending {
		t.Fatalf("status = %v, want %v", res.Status, payment.PaymentPending)
	}

	if err := p.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}
}

func TestNew_autoApprove(t *testing.T) {
	t.Parallel()

	p, err := New(payment.Options{AutoApprove: true})
	if err != nil {
		t.Fatalf("New err = %v", err)
	}

	res, err := p.CreatePayment(t.Context(), payment.Request{Amount: 10, Currency: "USD"})
	if err != nil {
		t.Fatalf("CreatePayment err = %v", err)
	}

	if res.Status != payment.PaymentSucceeded {
		t.Fatalf("status = %v, want %v", res.Status, payment.PaymentSucceeded)
	}

	if err := p.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}
}

func TestNew_invalidOpts(t *testing.T) {
	t.Parallel()

	_, err := New(payment.Options{MaxWebhookBytes: -1})
	if !errors.Is(err, payment.ErrInvalidOptions) {
		t.Fatalf("New err = %v, want ErrInvalidOptions", err)
	}
}

func TestWebhookEvent_static(t *testing.T) {
	t.Parallel()

	p := newStub(t, true)

	if _, err := p.WebhookEvent(t.Context(), []byte(`{}`), "sig"); !errors.Is(err, payment.ErrInvalidSignature) {
		t.Fatalf("WebhookEvent err = %v, want ErrInvalidSignature", err)
	}
}

func TestClose_nil(t *testing.T) {
	t.Parallel()

	if err := newStub(t, true).Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}
}

func TestConcurrent_createRefund(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	p := newStub(t, true)

	var wg sync.WaitGroup

	for range 50 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			res, err := p.CreatePayment(ctx, payment.Request{Amount: 100, Currency: "USD"})
			if err != nil {
				t.Errorf("CreatePayment err = %v", err)
				return
			}

			if err := p.Refund(ctx, res.ID, 100, ""); err != nil {
				t.Errorf("Refund err = %v", err)
			}
		}()
	}

	wg.Wait()
}
