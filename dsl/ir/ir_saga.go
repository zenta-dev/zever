package ir

import "github.com/zenta-dev/zever/dsl/diag"

// Saga represents a top-level saga declaration: an ordered composition of
// service RPCs into a compensatable workflow. Steps execute in declaration
// order; a failure after the pivot step rolls forward instead of
// compensating.
type Saga struct {
	Name string
	// Steps are the saga's steps in declaration order.
	Steps []*SagaStep
	// Module is the module the saga was declared in. A step may reference
	// an RPC of a service declared in any other module (cross-module
	// references are the point of a saga, mirroring how schedules may
	// dispatch jobs across modules).
	Module *Module
	Pos    diag.Position
	// DocComment is the "//" comment written immediately above this saga's
	// declaration in the source .zen file, or "" if there was none.
	DocComment string
}

// SagaStep is one forward action in a saga plus its optional reverse
// compensation.
type SagaStep struct {
	// Name is the step's unique-within-the-saga identifier.
	Name string
	// Execute is the forward RPC. Always non-nil on a resolved step; the
	// resolver rejects a step that declares no execute option.
	Execute ServiceRPC
	// Compensate is the compensating RPC, run when a later step fails and
	// this step must be undone. Nil when the step declares none.
	Compensate *ServiceRPC
	// Pivot marks the step after which failures roll forward (retry the
	// remaining steps) instead of compensating. At most one step per saga
	// may set it.
	Pivot bool
	Pos   diag.Position
}

// ServiceRPC is a resolved reference to one operation of one service,
// naming both so a generated saga step can invoke it by name.
type ServiceRPC struct {
	Service *Service
	RPC     *Operation
	Pos     diag.Position
}
