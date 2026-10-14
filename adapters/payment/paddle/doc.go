// Package paddle provides a payment.Payment backed by Paddle transactions and adjustments.
//
// All amounts are minor units (cents). Paddle decimal totals arrive as
// major-unit strings and parse via the shared billing.ParseMinorUnits
// contract (whole units scale by the currency exponent, excess precision
// rounds half away from zero); partial-refund adjustment amounts encode back
// with billing.FormatMinorUnits.
package paddle
