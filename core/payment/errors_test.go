package payment

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func fmtWrap(err error) error {
	return fmt.Errorf("wrap: %w", err)
}

func TestSentinels_messages_prefixed(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
	}{
		{"nil factory", ErrNilFactory},
		{"duplicate", ErrDuplicateAdapter},
		{"unknown", ErrUnknownAdapter},
		{"invalid adapter", ErrInvalidAdapter},
		{"invalid options", ErrInvalidOptions},
		{"invalid amount", ErrInvalidAmount},
		{"missing currency", ErrMissingCurrency},
		{"unsupported method", ErrUnsupportedMethod},
		{"not found", ErrNotFound},
		{"missing payment id", ErrMissingPaymentID},
		{"webhook too large", ErrWebhookTooLarge},
		{"missing webhook secret", ErrMissingWebhookSecret},
		{"invalid signature", ErrInvalidSignature},
		{"duplicate event", ErrDuplicateEvent},
		{"amount mismatch", ErrAmountMismatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if !strings.HasPrefix(tc.err.Error(), "payment: ") {
				t.Errorf("sentinel %q missing prefix", tc.err.Error())
			}
		})
	}
}

func TestDuplicateAdapterError_message_unwrap(t *testing.T) {
	t.Parallel()

	err := DuplicateAdapterError{Adapter: "stub"}
	if got := err.Error(); !strings.Contains(got, ErrDuplicateAdapter.Error()) {
		t.Errorf("Error() = %q, want contain %q", got, ErrDuplicateAdapter.Error())
	}

	if !errors.Is(err, ErrDuplicateAdapter) {
		t.Errorf("errors.Is(%v, ErrDuplicateAdapter) = false, want true", err)
	}

	if _, ok := errors.AsType[DuplicateAdapterError](fmtWrap(err)); !ok {
		t.Errorf("errors.As failed for %T", err)
	}
}

func TestDuplicateAliases_compat(t *testing.T) {
	t.Parallel()

	err := DuplicateAdapterError{Adapter: "stub"}
	if !errors.Is(err, ErrDuplicate) {
		t.Errorf("errors.Is(%v, ErrDuplicate) = false, want true", err)
	}

	if _, ok := errors.AsType[DuplicateError](fmtWrap(err)); !ok {
		t.Errorf("errors.As failed for %T alias", err)
	}

	if !errors.Is(ErrDuplicateAdapter, ErrDuplicate) {
		t.Errorf("alias ErrDuplicate does not match ErrDuplicateAdapter")
	}
}

func TestUnknownAdapterError_message_unwrap(t *testing.T) {
	t.Parallel()

	err := UnknownAdapterError{Adapter: "stripe"}
	if got := err.Error(); !strings.Contains(got, ErrUnknownAdapter.Error()) {
		t.Errorf("Error() = %q, want contain %q", got, ErrUnknownAdapter.Error())
	}

	if !errors.Is(err, ErrUnknownAdapter) {
		t.Errorf("errors.Is(%v, ErrUnknownAdapter) = false, want true", err)
	}

	if _, ok := errors.AsType[UnknownAdapterError](fmtWrap(err)); !ok {
		t.Errorf("errors.As failed for %T", err)
	}
}

func TestInvalidAdapterError_message_unwrap(t *testing.T) {
	t.Parallel()

	err := InvalidAdapterError{Adapter: "bogus"}
	if got := err.Error(); !strings.Contains(got, ErrInvalidAdapter.Error()) {
		t.Errorf("Error() = %q, want contain %q", got, ErrInvalidAdapter.Error())
	}

	if !strings.Contains(err.Error(), "bogus") {
		t.Errorf("Error() = %q, want contain adapter name", err.Error())
	}

	if !errors.Is(err, ErrInvalidAdapter) {
		t.Errorf("errors.Is(%v, ErrInvalidAdapter) = false, want true", err)
	}

	if _, ok := errors.AsType[InvalidAdapterError](fmtWrap(err)); !ok {
		t.Errorf("errors.As failed for %T", err)
	}
}

func TestInvalidOptionsError_message_unwrap(t *testing.T) {
	t.Parallel()

	err := InvalidOptionsError{Reason: "boom"}
	if got := err.Error(); !strings.Contains(got, ErrInvalidOptions.Error()) {
		t.Errorf("Error() = %q, want contain %q", got, ErrInvalidOptions.Error())
	}

	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("Error() = %q, want contain reason", err.Error())
	}

	if !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("errors.Is(%v, ErrInvalidOptions) = false, want true", err)
	}

	if _, ok := errors.AsType[InvalidOptionsError](fmtWrap(err)); !ok {
		t.Errorf("errors.As failed for %T", err)
	}
}

func TestNotFoundError_message_unwrap(t *testing.T) {
	t.Parallel()

	err := NotFoundError{PaymentID: "pay_123"}
	if got := err.Error(); !strings.Contains(got, ErrNotFound.Error()) {
		t.Errorf("Error() = %q, want contain %q", got, ErrNotFound.Error())
	}

	if !strings.Contains(err.Error(), "pay_123") {
		t.Errorf("Error() = %q, want contain payment id", err.Error())
	}

	if !errors.Is(err, ErrNotFound) {
		t.Errorf("errors.Is(%v, ErrNotFound) = false, want true", err)
	}

	if _, ok := errors.AsType[NotFoundError](fmtWrap(err)); !ok {
		t.Errorf("errors.As failed for %T", err)
	}
}

func TestUnsupportedMethodError_message_unwrap(t *testing.T) {
	t.Parallel()

	err := UnsupportedMethodError{Method: PaymentMethod("crypto")}
	if got := err.Error(); !strings.Contains(got, ErrUnsupportedMethod.Error()) {
		t.Errorf("Error() = %q, want contain %q", got, ErrUnsupportedMethod.Error())
	}

	if !strings.Contains(err.Error(), "crypto") {
		t.Errorf("Error() = %q, want contain method", err.Error())
	}

	if !errors.Is(err, ErrUnsupportedMethod) {
		t.Errorf("errors.Is(%v, ErrUnsupportedMethod) = false, want true", err)
	}

	if _, ok := errors.AsType[UnsupportedMethodError](fmtWrap(err)); !ok {
		t.Errorf("errors.As failed for %T", err)
	}
}

func TestAmountMismatchError_message_unwrap(t *testing.T) {
	t.Parallel()

	err := AmountMismatchError{Expected: 100, Actual: 90}
	if got := err.Error(); !strings.Contains(got, ErrAmountMismatch.Error()) {
		t.Errorf("Error() = %q, want contain %q", got, ErrAmountMismatch.Error())
	}

	if !errors.Is(err, ErrAmountMismatch) {
		t.Errorf("errors.Is(%v, ErrAmountMismatch) = false, want true", err)
	}

	target, ok := errors.AsType[AmountMismatchError](fmtWrap(err))
	if !ok {
		t.Errorf("errors.As failed for %T", err)
	}

	if target.Expected != 100 || target.Actual != 90 {
		t.Errorf("carried values = {%d %d}, want {100 90}", target.Expected, target.Actual)
	}
}

func TestSizeLimitError_message_unwrap(t *testing.T) {
	t.Parallel()

	err := SizeLimitError{Size: 100, Limit: 10}
	if got := err.Error(); !strings.Contains(got, ErrWebhookTooLarge.Error()) {
		t.Errorf("Error() = %q, want contain %q", got, ErrWebhookTooLarge.Error())
	}

	if !errors.Is(err, ErrWebhookTooLarge) {
		t.Errorf("errors.Is(%v, ErrWebhookTooLarge) = false, want true", err)
	}

	if _, ok := errors.AsType[SizeLimitError](fmtWrap(err)); !ok {
		t.Errorf("errors.As failed for %T", err)
	}
}

func TestDuplicateEventError_message_unwrap(t *testing.T) {
	t.Parallel()

	err := DuplicateEventError{EventID: "evt_123"}
	if got := err.Error(); !strings.Contains(got, ErrDuplicateEvent.Error()) {
		t.Errorf("Error() = %q, want contain %q", got, ErrDuplicateEvent.Error())
	}

	if !strings.Contains(err.Error(), "evt_123") {
		t.Errorf("Error() = %q, want contain event id", err.Error())
	}

	if !errors.Is(err, ErrDuplicateEvent) {
		t.Errorf("errors.Is(%v, ErrDuplicateEvent) = false, want true", err)
	}

	target, ok := errors.AsType[DuplicateEventError](fmtWrap(err))
	if !ok {
		t.Errorf("errors.As failed for %T", err)
	}

	if target.EventID != "evt_123" {
		t.Errorf("carried event id = %q, want evt_123", target.EventID)
	}
}
