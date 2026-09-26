package ast

import (
	"time"

	"github.com/zenta-dev/zever/dsl/diag"
)

// Value is the interface implemented by all value expressions.
type Value interface {
	valuePos() diag.Position
}

// StringLit represents a string literal value.
type StringLit struct {
	Pos   diag.Position
	Value string
}

func (v *StringLit) valuePos() diag.Position { return v.Pos }

// IntLit represents an integer literal value.
type IntLit struct {
	Pos   diag.Position
	Value int64
}

func (v *IntLit) valuePos() diag.Position { return v.Pos }

// FloatLit represents a floating-point literal value.
type FloatLit struct {
	Pos   diag.Position
	Value float64
}

func (v *FloatLit) valuePos() diag.Position { return v.Pos }

// DurationLit represents a duration literal value.
type DurationLit struct {
	Pos   diag.Position
	Raw   string
	Value time.Duration
	Valid bool
}

func (v *DurationLit) valuePos() diag.Position { return v.Pos }

// IdentValue represents an identifier value.
type IdentValue struct {
	Pos  diag.Position
	Name string
}

func (v *IdentValue) valuePos() diag.Position { return v.Pos }

// CallValue represents a function call value.
type CallValue struct {
	Pos     diag.Position
	NamePos diag.Position
	Name    string
	Args    []*Arg
}

func (v *CallValue) valuePos() diag.Position { return v.Pos }

// SetLit represents a set literal value.
type SetLit struct {
	Pos     diag.Position
	Items   []string
	ItemPos []diag.Position
}

func (v *SetLit) valuePos() diag.Position { return v.Pos }

// Attribute represents an attribute annotation with arguments.
type Attribute struct {
	Pos     diag.Position
	NamePos diag.Position
	Name    string
	Args    []*Arg
}

// Arg represents an argument to a function call or attribute.
type Arg struct {
	Pos   diag.Position
	Name  string
	Value Value
}
