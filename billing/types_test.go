package billing

import "testing"

func TestSubscriptionStatus_values(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		status SubscriptionStatus
		want   string
	}{
		{"active", SubscriptionActive, "active"},
		{"canceled", SubscriptionCanceled, "canceled"},
		{"past due", SubscriptionPastDue, "past_due"},
		{"trial", SubscriptionTrial, "trial"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if string(tc.status) != tc.want {
				t.Errorf("SubscriptionStatus = %q, want %q", string(tc.status), tc.want)
			}
		})
	}
}

func TestInvoiceStatus_values(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		status InvoiceStatus
		want   string
	}{
		{"paid", InvoicePaid, "paid"},
		{"draft", InvoiceDraft, "draft"},
		{"finalized", InvoiceFinalized, "finalized"},
		{"open", InvoiceOpen, "open"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if string(tc.status) != tc.want {
				t.Errorf("InvoiceStatus = %q, want %q", string(tc.status), tc.want)
			}
		})
	}
}
