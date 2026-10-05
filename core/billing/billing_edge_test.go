package billing

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// nilCtx is a nil context used to prove the battery never dereferences ctx
// before the adapter boundary.
var nilCtx context.Context

func TestEdgeOpen_emptyAdapterName(t *testing.T) {
	t.Parallel()

	got, err := Open(Adapter(""), Options{})
	if !errors.Is(err, ErrUnknownAdapter) {
		t.Fatalf("Open(\"\") err = %v, want ErrUnknownAdapter", err)
	}
	var ue UnknownAdapterError
	if !errors.As(err, &ue) {
		t.Fatalf("err %T is not *UnknownAdapterError", err)
	}
	if ue.Adapter != Adapter("") {
		t.Fatalf("Adapter = %v, want empty", ue.Adapter)
	}
	if got != nil {
		t.Fatalf("got = %v, want nil", got)
	}
}

func TestEdgeOpen_factoryReturnsNilNil(t *testing.T) {
	t.Parallel()

	a := freshAdapter()
	var nilBilling Billing
	if err := Register(a, func(Options) (Billing, error) { return nilBilling, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	got, err := Open(a, Options{})
	if err != nil {
		t.Fatalf("Open err = %v, want nil", err)
	}
	if got != nil {
		t.Fatalf("got = %v, want nil", got)
	}
}

func TestEdgeParseMinorUnits_zero(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
		want  int64
	}{
		{"zero whole", "0", 0},
		{"zero decimal", "0.00", 0},
		{"negative cent", "-0.01", -1},
		{"half up at zero", "0.005", 1},
		{"rounds up", "1.999", 200},
		{"long fraction truncates then rounds", "1.999999", 200},
		{"plus with spaces", "  +10.00  ", 1000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseMinorUnits(tc.input, "usd")
			if err != nil {
				t.Fatalf("ParseMinorUnits(%q) err = %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("ParseMinorUnits(%q) = %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}

func TestEdgeParseMinorUnits_int64Boundaries(t *testing.T) {
	t.Parallel()

	// The overflow guard is conservative: whole must stay <= MaxInt64-1 even
	// for zero-exponent currencies.
	got, err := ParseMinorUnits("9223372036854775806", "jpy")
	if err != nil {
		t.Fatalf("ParseMinorUnits(MaxInt64-1, jpy) err = %v", err)
	}
	if got != 9223372036854775806 {
		t.Fatalf("ParseMinorUnits(MaxInt64-1, jpy) = %d, want MaxInt64-1", got)
	}

	// MaxInt64 itself trips the guard.
	_, err = ParseMinorUnits("9223372036854775807", "jpy")
	if !errors.Is(err, ErrAmountOverflow) {
		t.Fatalf("ParseMinorUnits(MaxInt64, jpy) err = %v, want ErrAmountOverflow", err)
	}
}

func TestEdgeOptionsValidate_endpointQueryAndFragment(t *testing.T) {
	t.Parallel()

	cases := []string{
		"https://example.com/hook?a=b",
		"https://example.com/hook#frag",
	}
	for _, endpoint := range cases {
		if err := (Options{Endpoint: endpoint}).Validate(); err != nil {
			t.Errorf("Validate(%q) err = %v, want nil", endpoint, err)
		}
	}
}

func TestEdgeCreateCustomer_nilContext(t *testing.T) {
	t.Parallel()

	a := freshAdapter()
	stub := &stubBilling{}
	if err := Register(a, func(Options) (Billing, error) { return stub, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	got, err := Open(a, Options{})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	if _, err := got.CreateCustomer(nilCtx, "Ada", "ada@example.com", ""); err != nil {
		t.Fatalf("CreateCustomer(nil ctx) err = %v, want nil", err)
	}
	if _, err := got.CreateSubscription(nilCtx, "cus_1", "plan_pro", ""); err != nil {
		t.Fatalf("CreateSubscription(nil ctx) err = %v, want nil", err)
	}
	if err := got.CancelSubscription(nilCtx, "sub_1"); err != nil {
		t.Fatalf("CancelSubscription(nil ctx) err = %v, want nil", err)
	}
	if _, err := got.GetInvoice(nilCtx, "cus_1"); err != nil {
		t.Fatalf("GetInvoice(nil ctx) err = %v, want nil", err)
	}
}

func TestEdgeNotFoundError_message(t *testing.T) {
	t.Parallel()

	err := NotFoundError{Resource: "customer", ID: "cus_1"}
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if !strings.Contains(err.Error(), "customer") || !strings.Contains(err.Error(), "cus_1") {
		t.Fatalf("Error() = %q, want resource and id", err.Error())
	}
}

func TestEdgeParseAdapter_whitespace(t *testing.T) {
	t.Parallel()

	got, err := ParseAdapter(" ")
	if err != nil {
		t.Fatalf("ParseAdapter(\" \") err = %v, want nil", err)
	}
	if got != Adapter(" ") {
		t.Fatalf("ParseAdapter(\" \") = %v, want space", got)
	}
}
