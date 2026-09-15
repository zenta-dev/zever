package billing

import "strings"

// CurrencyExponent returns the number of minor units for the currency.
func CurrencyExponent(currency string) int {
	switch strings.ToLower(currency) {
	case "clf":
		return 4
	case "bhd", "iqd", "jod", "kwd", "lyd", "omr", "tnd":
		return 3
	case "bif", "clp", "djf", "gnf", "isk", "jpy", "kmf", "krw", "pyg", "rwf", "ugx", "vnd", "vuv", "xaf", "xof", "xpf":
		return 0
	default:
		return 2
	}
}
