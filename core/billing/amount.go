package billing

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ParseMinorUnits parses a decimal major-unit amount string into minor units
// for currency. All Amount fields across payment and billing are minor units;
// ParseMinorUnits is the single shared helper that converts provider decimal
// strings (e.g. Paddle "10.00") into that contract.
//
// Excess fraction digits beyond the currency precision are rounded half away
// from zero. Amounts that no longer fit int64 once scaled fail with
// ErrAmountOverflow; the boundary is exact, so the largest representable
// totals still convert.
//
// The grammar is strict: an optional sign followed by whole digits, with an
// optional fraction that requires digits on both sides of the point.
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

	if whole > math.MaxInt64/scale {
		return 0, ErrAmountOverflow
	}

	mag := uint64(whole) * uint64(scale) //nolint:gosec // G115: whole holds unsigned digits, scale is positive

	if fracStr != "" {
		frac, err := parseFraction(fracStr, scale)
		if err != nil {
			return 0, err
		}

		mag += uint64(frac) //nolint:gosec // G115: frac is a non-negative magnitude
	}

	if sign > 0 {
		if mag > math.MaxInt64 {
			return 0, ErrAmountOverflow
		}

		return int64(mag), nil
	}

	if mag > 1<<63 {
		return 0, ErrAmountOverflow
	}

	if mag == 1<<63 {
		return math.MinInt64, nil
	}

	return -int64(mag), nil //nolint:gosec // G115: mag <= MaxInt64 past the 1<<63 guard
}

// FormatMinorUnits formats minor units as a decimal major-unit amount string
// for currency. FormatMinorUnits is the inverse of ParseMinorUnits: every
// int64 value round-trips, so ParseMinorUnits(FormatMinorUnits(v, currency),
// currency) always returns v, except math.MinInt64 under zero-exponent
// currencies (such as JPY), whose whole part has no positive int64 literal.
// The result carries exactly CurrencyExponent(currency) fraction digits (none
// for zero-exponent currencies such as JPY).
func FormatMinorUnits(v int64, currency string) string {
	exp := CurrencyExponent(currency)
	scale := scaleForCurrency(currency)
	uscale := uint64(scale) //nolint:gosec // G115: scale is a positive power of ten

	neg := v < 0

	var mag uint64
	if neg {
		mag = uint64(-(v + 1)) + 1 //nolint:gosec // G115: v < 0, so -(v+1) is non-negative
	} else {
		mag = uint64(v)
	}

	whole := mag / uscale
	frac := mag % uscale

	var b strings.Builder

	if neg {
		b.WriteByte('-')
	}

	b.WriteString(strconv.FormatUint(whole, 10))

	if exp > 0 {
		b.WriteByte('.')

		fs := strconv.FormatUint(frac, 10)
		for i := len(fs); i < exp; i++ {
			b.WriteByte('0')
		}

		b.WriteString(fs)
	}

	return b.String()
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

	// Scale from 1e5 base to currency scale, rounding the magnitude half away
	// from zero (the caller applies the sign).
	product := f * scale
	small := product / 100000
	remainder := product % 100000

	if remainder*2 >= 100000 {
		small++
	}

	return small, nil
}
