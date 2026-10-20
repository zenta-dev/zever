// Package wire extracts JSON wire-contract facts from Go struct tags.
//
// The rules encoded here mirror encoding/json: a field without a json tag,
// or with a json tag whose name is empty, is encoded under its Go field
// name, and a field tagged json:"-" is not encoded at all. The struct-tag
// guard (tools/tagcheck) uses these facts to pin the wire key of every
// field of a registered wire struct.
package wire

import (
	"go/ast"
	"go/token"
	"reflect"
	"strings"
)

// State classifies a struct field's json tag for the wire contract.
type State int

const (
	// StateMissing means the field carries no json tag: encoding/json
	// encodes the field under its Go field name.
	StateMissing State = iota
	// StateEmpty means the field carries a json tag with an empty name
	// (json:"" or json:",omitempty"): encoding/json encodes the field
	// under its Go field name.
	StateEmpty
	// StateDropped means the field carries json:"-": encoding/json does
	// not encode the field at all.
	StateDropped
	// StateNamed means the field carries json:"name[,options]":
	// encoding/json encodes the field under name.
	StateNamed
)

// String returns the state name.
func (s State) String() string {
	switch s {
	case StateMissing:
		return "missing"
	case StateEmpty:
		return "empty"
	case StateDropped:
		return "dropped"
	case StateNamed:
		return "named"
	default:
		return "unknown"
	}
}

// Field is one exported struct field and its raw struct tag.
type Field struct {
	// Name is the Go field name.
	Name string
	// Tag is the raw struct tag text, without the surrounding backticks.
	Tag string
	// HasTag reports whether the field declares a struct tag at all.
	HasTag bool
}

// State returns the wire state of the field's json tag.
func (f Field) State() State {
	return Classify(f.Tag)
}

// Key returns the JSON object key the field is encoded under and whether
// the field is on the wire at all. encoding/json falls back to the Go
// field name when the json tag carries no name, so a field with a missing
// or empty json tag is encoded under its Go field name.
func (f Field) Key() (string, bool) {
	switch f.State() {
	case StateNamed:
		return tagKey(f.Tag), true
	case StateEmpty, StateMissing:
		return f.Name, true
	case StateDropped:
		return "-", false
	default:
		return "-", false
	}
}

// tagKey returns the wire key of a json struct tag: the name part before
// the first comma. It returns an empty string when the tag carries no json
// name.
func tagKey(tag string) string {
	value, ok := reflect.StructTag(tag).Lookup("json")
	if !ok {
		return ""
	}

	name, _, _ := strings.Cut(value, ",")

	return name
}

// Classify reports the wire state of a struct field's json tag.
func Classify(tag string) State {
	value, ok := reflect.StructTag(tag).Lookup("json")
	if !ok {
		return StateMissing
	}

	name, _, _ := strings.Cut(value, ",")

	switch name {
	case "":
		return StateEmpty
	case "-":
		return StateDropped
	default:
		return StateNamed
	}
}

// Fields returns the exported fields of the struct type named name declared
// in f. It reports false when f declares no struct type with that name.
// Embedded (anonymous) fields are skipped: encoding/json flattens them
// into the enclosing object, so they contribute no key of their own.
func Fields(f *ast.File, name string) ([]Field, bool) {
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}

		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok || ts.Name.Name != name {
				continue
			}

			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				return nil, false
			}

			return exportedFields(st), true
		}
	}

	return nil, false
}

// exportedFields returns the exported fields of st.
func exportedFields(st *ast.StructType) []Field {
	fields := make([]Field, 0, len(st.Fields.List))

	for _, field := range st.Fields.List {
		if len(field.Names) == 0 {
			continue // embedded fields carry no key of their own
		}

		for _, ident := range field.Names {
			if !ident.IsExported() {
				continue
			}

			fld := Field{Name: ident.Name}
			if field.Tag != nil {
				fld.Tag = strings.Trim(field.Tag.Value, "`")
				fld.HasTag = true
			}

			fields = append(fields, fld)
		}
	}

	return fields
}
