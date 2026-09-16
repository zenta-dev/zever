// Package apperror bridges business logic to HTTP and gRPC transports with a typed error vocabulary.
//
// Business logic returns typed errors carrying an ErrorCode, and generated
// transport wiring maps that Code to the right wire-level status via
// errors.As, without the business logic ever touching net/http or gRPC
// packages itself. Unknown codes degrade fail-closed to UNKNOWN and HTTP
// 500, never to success.
//
// The 16-value gRPC-canonical vocabulary is kept independent and
// dependency-free on purpose (stdlib only), so application code can import
// it directly at runtime in production.
package apperror
