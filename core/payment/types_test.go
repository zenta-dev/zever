package payment

import "testing"

func TestPaymentStatus_values(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		status PaymentStatus
		want   string
	}{
		{"succeeded", PaymentSucceeded, "succeeded"},
		{"pending", PaymentPending, "pending"},
		{"failed", PaymentFailed, "failed"},
		{"partially refunded", PaymentPartiallyRefunded, "partially_refunded"},
		{"refunded", PaymentRefunded, "refunded"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if string(tc.status) != tc.want {
				t.Errorf("PaymentStatus = %q, want %q", string(tc.status), tc.want)
			}
		})
	}
}

func TestPaymentMethod_values(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		method PaymentMethod
		want   string
	}{
		{"card", MethodCard, "card"},
		{"bank transfer", MethodBankTransfer, "bank_transfer"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if string(tc.method) != tc.want {
				t.Errorf("PaymentMethod = %q, want %q", string(tc.method), tc.want)
			}
		})
	}
}
