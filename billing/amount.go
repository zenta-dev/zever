package billing

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ParseMinorUnits parses a decimal amount string into minor units for currency.
// Excess fraction digits beyond the currency precision are rounded half-up.
func ParseMinorUnits(s string, currency string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, ErrEmptyAmount
	}

	sign, rest, err := splitSign(s)
	if err != nil {
		return 0, err
	}

	wholeStr, fracStr, err := splitDecimal(rest)
	if err != nil {
		return 0, err
	}

	whole, err := parseWhole(wholeStr)
	if err != nil {
		return 0, err
	}

	scale := scaleForCurrency(currency)

	if whole > (math.MaxInt64-scale)/scale {
		return 0, ErrAmountOverflow
	}

	minor := whole * scale

	if fracStr == "" {
		return sign * minor, nil
	}

	fracMinor, err := parseFraction(fracStr, scale)
	if err != nil {
		return 0, err
	}

	return sign * (minor + fracMinor), nil
}

func splitSign(s string) (int64, string, error) {
	switch s[0] {
	case '-':
		if len(s) == 1 {
			return 0, "", fmt.Errorf("%w: %q", ErrMalformedAmount, s)
		}

		return -1, s[1:], nil
	case '+':
		if len(s) == 1 {
			return 0, "", fmt.Errorf("%w: %q", ErrMalformedAmount, s)
		}

		return 1, s[1:], nil
	default:
		return 1, s, nil
	}
}

func splitDecimal(s string) (string, string, error) {
	idx := strings.IndexByte(s, '.')
	if idx < 0 {
		return s, "", nil
	}

	whole := s[:idx]
	frac := s[idx+1:]

	if whole == "" || frac == "" {
		return "", "", fmt.Errorf("%w: %q", ErrMalformedAmount, s)
	}

	return whole, frac, nil
}

func parseWhole(s string) (int64, error) {
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %q", ErrMalformedAmount, s)
	}

	return v, nil
}

func scaleForCurrency(currency string) int64 {
	exp := CurrencyExponent(currency)

	var scale int64 = 1
	for i := 0; i < exp; i++ {
		scale *= 10
	}

	return scale
}

// parseFraction converts fractional digits to minor units with rounding.
// frac is padded/truncated to 5 digits (100000) then scaled to the currency
// scale with half-up rounding.
func parseFraction(frac string, scale int64) (int64, error) {
	if len(frac) > 5 {
		frac = frac[:5]
	}

	for len(frac) < 5 {
		frac += "0"
	}

	f, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %q", ErrMalformedAmount, frac)
	}

	// Scale from 1e5 base to currency scale with half-up rounding.
	product := f * scale
	small := product / 100000
	remainder := product % 100000

	if remainder*2 >= 100000 {
		small++
	}

	return small, nil
}
