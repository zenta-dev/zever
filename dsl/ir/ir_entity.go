package ir

import "github.com/zenta-dev/zever/dsl/diag"

// Validation represents a validation rule on a field.
// Kind: format|min_len|max_len|gt|gte|lt|lte.
type Validation struct {
	Kind string         // The validation kind
	Args map[string]any // Arguments for the validation rule
	Pos  diag.Position  // Source position
}

// DefaultValue represents a default value for a field.
// Kind: literal|now.
type DefaultValue struct {
	Kind string        // The default kind
	Lit  any           // The literal value or nil for "now"
	Pos  diag.Position // Source position
}

// Field represents a field in an entity.
type Field struct {
	Name     string
	Type     FieldType
	Primary  bool
	Unique   bool
	Optional bool
	Validate []Validation
	Default  *DefaultValue
	// RenamedFrom is the column's previous name, recorded by
	// @renamed_from("old_name"), or nil when the field was never renamed.
	// A rename is indistinguishable from "drop the old column, add a new
	// one" under a live-vs-declared diff, so migration tooling can only
	// know about it because the schema says so explicitly.
	RenamedFrom *string
	// Ref is non-nil when this field references an Entity or Message by
	// name (the same TypeRef union ir.Param.Ref and Operation.Returns
	// already use) instead of a scalar FieldType. Only ever set on a
	// Message field (resolver_message.go's resolveFieldForMessage) --
	// entity field resolution never sets it, so Type is meaningless and
	// unused when Ref is set. Backends that only ever walk entity fields
	// (atlas, zenorm) can assume Ref == nil unconditionally.
	Ref *TypeRef
	Pos diag.Position
	// DocComment is the "//" comment written immediately above this field's
	// declaration in the source .zen file, or "" if there was none. See
	// ast.FieldDecl.DocComment and parser.docCommentFor for the exact
	// attachment rule.
	DocComment string
}

// RelationKind represents the type of relationship between entities.
type RelationKind int

const (
	//nolint:revive
	HasMany RelationKind = iota
	//nolint:revive
	HasOne
	//nolint:revive
	BelongsTo
	//nolint:revive
	ManyToMany
)

// Relation represents a relationship between entities.
type Relation struct {
	Kind       RelationKind
	FieldName  string    // The field name on this entity
	Target     *Entity   // Resolved pointer to target entity
	ForeignKey *Field    // Resolved pointer to foreign key field
	OnDelete   string    // "" | restrict | cascade | set_null
	JoinTable  string    // many_to_many only
	Reciprocal *Relation // many_to_many only
	Pos        diag.Position
}

// Index represents a database index on an entity.
type Index struct {
	Columns []*Field // The columns in the index
	Unique  bool     // Whether the index is unique
	Pos     diag.Position
}

// Entity represents an entity (table) in the schema.
type Entity struct {
	Name      string
	Module    *Module // Resolved pointer; always non-nil
	Schema    string  // Postgres schema: explicit @schema(x), else Module.Name, else "public"
	Fields    []*Field
	Relations []*Relation
	Indexes   []*Index
	Pos       diag.Position
	// DocComment is the "//" comment written immediately above this
	// entity's declaration in the source .zen file, or "" if there was
	// none.
	DocComment string
}

// FieldByName returns the field with the given name, or nil if not found.
func (e *Entity) FieldByName(name string) *Field {
	if e == nil || e.Fields == nil {
		return nil
	}

	for _, f := range e.Fields {
		if f.Name == name {
			return f
		}
	}

	return nil
}
