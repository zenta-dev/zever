package ir

import "github.com/zenta-dev/zever/dsl/diag"

// Message represents a non-persistent DTO used only in service payloads.
type Message struct {
	Name   string
	Module *Module // Resolved pointer; always non-nil
	Fields []*Field
	Pos    diag.Position
	// DocComment is the "//" comment written immediately above this
	// message's declaration in the source .zen file, or "" if there was
	// none.
	DocComment string
}

// FieldByName returns the field with the given name, or nil if not found.
func (m *Message) FieldByName(name string) *Field {
	if m == nil || m.Fields == nil {
		return nil
	}

	for _, f := range m.Fields {
		if f.Name == name {
			return f
		}
	}

	return nil
}
