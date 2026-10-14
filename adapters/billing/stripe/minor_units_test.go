package stripe

import (
	"net/http/httptest"
	"testing"
)

// TestInvoiceMinorUnits proves the Stripe billing adapter passes amount_due
// through unscaled: the provider's 2000 arrives as 2000 minor units.
func TestInvoiceMinorUnits(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(newDefaultMux(nil))
	defer srv.Close()

	inv, err := openWithServer(t, srv).GetInvoice(t.Context(), "cus_1")
	if err != nil {
		t.Fatalf("GetInvoice: %v", err)
	}

	if inv.AmountDue != 2000 {
		t.Fatalf("AmountDue = %d, want 2000 minor units", inv.AmountDue)
	}

	if inv.Currency != "usd" {
		t.Fatalf("Currency = %q, want usd", inv.Currency)
	}
}
