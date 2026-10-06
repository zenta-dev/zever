package ast

import "github.com/zenta-dev/zever/dsl/diag"

// SagaDecl represents a top-level saga declaration: an ordered composition
// of service RPCs into a compensatable workflow.
type SagaDecl struct {
	Pos     diag.Position
	NamePos diag.Position
	Name    string
	Steps   []*SagaStepDecl
	// DocComment holds the "//" comment(s) written immediately above this
	// declaration, if any -- see parser.docCommentFor. Empty when there is
	// none.
	DocComment string
}

func (d *SagaDecl) declPos() diag.Position { return d.Pos }

// SagaStepDecl represents one step of a saga: the RPC to execute forward
// and, optionally, the RPC that undoes it.
type SagaStepDecl struct {
	Pos diag.Position
	// Name is the step's unique-within-the-saga identifier.
	Name string
	// Execute is the forward RPC reference. Required: a saga step with no
	// execute option is a parse error.
	Execute *RPCRef
	// Compensate is the compensating RPC reference, run when a later step
	// fails and this step must be undone. Nil when the step declares none.
	Compensate *RPCRef
	// Pivot marks the step after which failures roll forward (retry the
	// remaining steps) instead of compensating. At most one step per saga
	// may set it.
	Pivot bool
}

// RPCRef is a Service.RPC reference: an identifier, a dot, and another
// identifier, naming a service and one of its operations declared anywhere
// in the same schema (cross-module allowed).
type RPCRef struct {
	Pos     diag.Position
	Service string
	RPC     string
}
