// Package paddle provides a billing.Billing backed by Paddle customers and automatic transactions.
//
// All amounts are minor units (cents). Paddle decimal totals arrive as
// major-unit strings and parse via the shared billing.ParseMinorUnits
// contract (whole units scale by the currency exponent, excess precision
// rounds half away from zero).
package paddle
