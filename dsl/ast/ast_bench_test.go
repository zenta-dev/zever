package ast

import (
	"testing"

	"github.com/zenta-dev/zever/dsl/diag"
)

// BenchmarkRelationKindString measures the RelationKind.String() switch,
// including its invalid-kind default branch via a mixed kind sequence.
func BenchmarkRelationKindString(b *testing.B) {
	kinds := []RelationKind{HasMany, HasOne, BelongsTo, ManyToMany, RelationKind(42)}

	b.ReportAllocs()
	for b.Loop() {
		for _, k := range kinds {
			_ = k.String()
		}
	}
}

// BenchmarkDeclPos measures declPos across every Decl implementation; the
// parser and resolver both rely on this for source attribution.
func BenchmarkDeclPos(b *testing.B) {
	pos := diag.Position{File: "bench.zen", Line: 1, Col: 1}
	decls := []Decl{
		&EntityDecl{Pos: pos},
		&EnumDecl{Pos: pos},
		&JobDecl{Pos: pos},
		&MessageDecl{Pos: pos},
		&ScheduleDecl{Pos: pos},
		&ServiceDecl{Pos: pos},
	}

	b.ReportAllocs()
	for b.Loop() {
		for _, d := range decls {
			_ = d.declPos()
		}
	}
}

// BenchmarkValuePos measures valuePos across every Value implementation.
func BenchmarkValuePos(b *testing.B) {
	pos := diag.Position{File: "bench.zen", Line: 1, Col: 1}
	values := []Value{
		&StringLit{Pos: pos},
		&IntLit{Pos: pos},
		&FloatLit{Pos: pos},
		&DurationLit{Pos: pos},
		&IdentValue{Pos: pos},
		&CallValue{Pos: pos},
		&SetLit{Pos: pos},
	}

	b.ReportAllocs()
	for b.Loop() {
		for _, v := range values {
			_ = v.valuePos()
		}
	}
}

// BenchmarkNewFileDecls measures the append pattern every compiler phase
// uses to accumulate declarations into an ast.File.
func BenchmarkNewFileDecls(b *testing.B) {
	decls := make([]Decl, 0, 64)
	for b.Loop() {
		decls = decls[:0]
		for i := 0; i < 64; i++ {
			decls = append(decls, &EntityDecl{Name: "User"})
		}
		_ = &File{Name: "bench.zen", Decls: decls}
	}
}
