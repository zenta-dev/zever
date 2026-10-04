package postgres

import (
	"errors"

	"github.com/zenta-dev/zever/shared/dbconn"
)

// ErrLeaseHeld is returned when a slot lease is held by a live owner.
var ErrLeaseHeld = dbconn.ErrLeaseHeld

// ErrUnsupportedType is returned when a scanned column has an unexpected Go type.
var ErrUnsupportedType = errors.New("postgres: unsupported type")

// ErrInvalidSpecLength indicates invalid spec length.
var ErrInvalidSpecLength = errors.New("postgres: spec length must be 1-256")

// LeaseHeldError reports a slot claim against a lease held by a live owner.
type LeaseHeldError = dbconn.LeaseHeldError
