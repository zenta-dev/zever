package billing

import (
	"errors"
	"strings"
	"testing"
)

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

	// The overflow guard is exact: the largest representable whole still
	// converts, even for zero-exponent currencies.
	got, err := ParseMinorUnits("9223372036854775806", "jpy")
	if err != nil {
		t.Fatalf("ParseMinorUnits(MaxInt64-1, jpy) err = %v", err)
	}
	if got != 9223372036854775806 {
		t.Fatalf("ParseMinorUnits(MaxInt64-1, jpy) = %d, want MaxInt64-1", got)
	}

	got, err = ParseMinorUnits("9223372036854775807", "jpy")
	if err != nil {
		t.Fatalf("ParseMinorUnits(MaxInt64, jpy) err = %v", err)
	}
	if got != 9223372036854775807 {
		t.Fatalf("ParseMinorUnits(MaxInt64, jpy) = %d, want MaxInt64", got)
	}

	// One past the limit trips the guard.
	_, err = ParseMinorUnits("9223372036854775808", "jpy")
	if !errors.Is(err, ErrMalformedAmount) && !errors.Is(err, ErrAmountOverflow) {
		t.Fatalf("ParseMinorUnits(2^63, jpy) err = %v, want a range error", err)
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
