package redisclient

import (
	"github.com/zenta-dev/zever/shared/redisopt"
)

// Sentinel errors returned and wrapped by the redis package.
var (
	ErrInvalidAddress    = redisopt.ErrInvalidAddress
	ErrParseAddress      = redisopt.ErrParseAddress
	ErrCloseClient       = redisopt.ErrCloseClient
	ErrPlaintextRejected = redisopt.ErrPlaintextRejected
)

// InvalidAddressError describes a failure to validate or parse a Redis address.
type InvalidAddressError = redisopt.InvalidAddressError

// PlaintextRejectedError reports that RequireTLS is set on Options but the
// resolved address would connect without TLS.
type PlaintextRejectedError = redisopt.PlaintextRejectedError
