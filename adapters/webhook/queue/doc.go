// Package queue provides at-least-once webhook delivery via a durable queue
// with retry and dead-letter.
//
// Queued messages carry an X-Webhook-Signature header of the form
// "t=<unix-timestamp>,v1=<hex-hmac>", the hex-hmac being HMAC-SHA256 over
// "<unix-timestamp>.<payload>" (see buildSignatureHeader). Before a message
// is delivered, verifySignatureHeader recomputes that HMAC and compares it
// with hmac.Equal, then separately checks the timestamp against
// webhook.Options.ReplayTolerance (default 5 minutes). A MAC mismatch and an
// expired timestamp are reported through distinct sentinel errors,
// ErrSignatureMismatch and ErrSignatureExpired respectively, so a tampered
// payload can be told apart from an expired or replayed one.
package queue
