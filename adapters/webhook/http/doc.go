// Package http provides synchronous webhook delivery over HTTP with retry and
// a timestamped HMAC signature.
//
// Each delivery with a non-empty secret carries an X-Hub-Signature-256 header
// of the form "t=<unix-timestamp>,v1=<hex-hmac>", where the hex-hmac is
// HMAC-SHA256 over "<unix-timestamp>.<payload>". The timestamp lets a
// receiving verifier reject stale or replayed deliveries, not just tampered
// ones; see webhook/queue's verifySignatureHeader for the reference verifier
// and webhook.Options.ReplayTolerance for the acceptable clock-skew window.
package http
