package ast

import (
	"testing"

	"github.com/zenta-dev/zever/internal/dsl/diag"
)

func TestRelationKindString(t *testing.T) {
	tests := []struct {
		name     string
		kind     RelationKind
		expected string
	}{
		{
			name:     "HasMany",
			kind:     HasMany,
			expected: "HasMany",
		},
		{
			name:     "HasOne",
			kind:     HasOne,
			expected: "HasOne",
		},
		{
			name:     "BelongsTo",
			kind:     BelongsTo,
			expected: "BelongsTo",
		},
		{
			name:     "ManyToMany",
			kind:     ManyToMany,
			expected: "ManyToMany",
		},
		{
			name:     "invalid kind returns empty string",
			kind:     RelationKind(999),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.kind.String()
			if result != tt.expected {
				t.Fatalf("RelationKind.String() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestDeclPosReturnsPos covers every Decl implementation's declPos: each
// declaration reports the position it was parsed at, which downstream phases
// (resolver diagnostics, formatter) rely on for source attribution.
func TestDeclPosReturnsPos(t *testing.T) {
	pos := diag.Position{File: "test.zen", Line: 3, Col: 7}

	decls := []struct {
		name string
		decl Decl
	}{
		{"entity", &EntityDecl{Pos: pos}},
		{"enum", &EnumDecl{Pos: pos}},
		{"job", &JobDecl{Pos: pos}},
		{"message", &MessageDecl{Pos: pos}},
		{"schedule", &ScheduleDecl{Pos: pos}},
		{"service", &ServiceDecl{Pos: pos}},
	}

	for _, tt := range decls {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.decl.declPos(); got != pos {
				t.Fatalf("declPos() = %v, want %v", got, pos)
			}
		})
	}
}

// TestValuePosReturnsPos covers every Value implementation's valuePos: every
// value node carries its source position for error messages.
func TestValuePosReturnsPos(t *testing.T) {
	pos := diag.Position{File: "test.zen", Line: 5, Col: 2}

	values := []struct {
		name  string
		value Value
	}{
		{"string", &StringLit{Pos: pos}},
		{"int", &IntLit{Pos: pos}},
		{"float", &FloatLit{Pos: pos}},
		{"duration", &DurationLit{Pos: pos}},
		{"ident", &IdentValue{Pos: pos}},
		{"call", &CallValue{Pos: pos}},
		{"set", &SetLit{Pos: pos}},
	}

	for _, tt := range values {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.value.valuePos(); got != pos {
				t.Fatalf("valuePos() = %v, want %v", got, pos)
			}
		})
	}
}
