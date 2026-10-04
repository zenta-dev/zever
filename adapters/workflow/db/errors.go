package db

import (
	"github.com/zenta-dev/zever/shared/dbconn"
)

// ErrLeaseHeld is returned by Reclaim when another live owner holds the run.
var ErrLeaseHeld = dbconn.ErrLeaseHeld

// LeaseHeldError reports a reclaim against a lease held by another owner.
type LeaseHeldError = dbconn.LeaseHeldError
