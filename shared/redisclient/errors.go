package redisclient

import (
	"github.com/zenta-dev/zever/shared/redisopt"
)

// ErrInvalidAddress indicates an invalid Redis address.
var ErrInvalidAddress = redisopt.ErrInvalidAddress

// ErrParseAddress indicates a Redis address parse failure.
var ErrParseAddress = redisopt.ErrParseAddress

// ErrCloseClient indicates a Redis client close failure.
var ErrCloseClient = redisopt.ErrCloseClient

// ErrPlaintextRejected indicates a plaintext connection was rejected.
var ErrPlaintextRejected = redisopt.ErrPlaintextRejected

// InvalidAddressError describes a failure to validate or parse a Redis address.
type InvalidAddressError = redisopt.InvalidAddressError

// PlaintextRejectedError reports that RequireTLS is set on Options but the
// resolved address would connect without TLS.
type PlaintextRejectedError = redisopt.PlaintextRejectedError
