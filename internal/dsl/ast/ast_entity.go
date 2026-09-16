// Package ast defines abstract syntax tree nodes for the zen DSL.
package ast

import "github.com/zenta-dev/zever/internal/dsl/diag"

// Decl is the interface implemented by all top-level declarations.
type Decl interface {
	declPos() diag.Position
}

// File represents a zen language source file.
type File struct {
	Name  string
	Decls []Decl
}

// EntityDecl represents an entity declaration in the zen DSL.
type EntityDecl struct {
	Pos        diag.Position
	Name       string
	NamePos    diag.Position
	Attributes []*Attribute
	Fields     []*FieldDecl
	Relations  []*RelationDecl
	Indexes    []*IndexDecl
	// DocComment holds the "//" comment(s) written immediately above this
	// declaration, if any -- see parser.docCommentFor for the exact
	// attachment rule. Multiple contiguous "//" lines join with "\n". Empty
	// when there is no doc comment.
	DocComment string
}

func (d *EntityDecl) declPos() diag.Position { return d.Pos }

// FieldDecl represents a field declaration within an entity.
type FieldDecl struct {
	Pos        diag.Position
	NamePos    diag.Position
	Name       string
	Type       *TypeExpr
	Optional   bool
	Attributes []*Attribute
	// DocComment holds the "//" comment(s) written immediately above this
	// field, if any -- see parser.docCommentFor. Empty when there is none.
	DocComment string
}

// TypeExpr represents a type expression with optional arguments.
type TypeExpr struct {
	Pos     diag.Position
	NamePos diag.Position
	Name    string
	Args    []string
	ArgPos  []diag.Position
}

// RelationKind represents the kind of relationship between entities.
type RelationKind int

const (
	// HasMany represents a one-to-many relationship.
	HasMany RelationKind = iota
	// HasOne represents a one-to-one relationship.
	HasOne
	// BelongsTo represents a reverse one-to-many relationship.
	BelongsTo
	// ManyToMany represents a many-to-many relationship.
	ManyToMany
)

func (k RelationKind) String() string {
	switch k {
	case HasMany:
		return "HasMany"
	case HasOne:
		return "HasOne"
	case BelongsTo:
		return "BelongsTo"
	case ManyToMany:
		return "ManyToMany"
	default:
		return ""
	}
}

// RelationDecl represents a relationship declaration between entities.
type RelationDecl struct {
	Pos          diag.Position
	Kind         RelationKind
	FieldName    string
	Target       string
	FieldNamePos diag.Position
	TargetPos    diag.Position
	Attributes   []*Attribute
	Join         *JoinBlock
}

// JoinBlock represents a join table specification for many-to-many relationships.
type JoinBlock struct {
	Pos      diag.Position
	TablePos diag.Position
	Table    string
}

// IndexDecl represents an index declaration on entity fields.
type IndexDecl struct {
	Pos        diag.Position
	Columns    []string
	ColumnPos  []diag.Position
	Attributes []*Attribute
}
