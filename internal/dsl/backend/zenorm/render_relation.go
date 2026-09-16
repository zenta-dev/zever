package zenorm

// This file renders one ir.Relation's generated section: a package-level
// orm.Relation[Parent, Child] var, plus a Join<Entity><Relation>/
// LeftJoin<Entity><Relation> function pair wrapping orm.JoinOn/
// orm.LeftJoinOn -- the concrete typed join entry point for that relation,
// sparing a caller from spelling out Join2/LeftJoin2's four type parameters
// by hand. The orm package's own scanJoinRow/scanLeftJoinRow already do the
// actual per-row Scan-call plumbing generically for ANY Relation -- there is
// nothing relation-specific left for codegen to emit at the scanning layer
// itself, which is why this is a thin typed wrapper rather than a
// hand-unrolled scan function.

import (
	"fmt"
	"strings"
)

// renderRelations appends every entity's declared relations' generated
// section to b.
func renderRelations(b *strings.Builder, m entityModel) {
	for _, r := range m.Relations {
		renderRelation(b, m, r)
	}
}

// renderRelation emits one relation's orm.Relation var and its
// Join/LeftJoin helper pair.
func renderRelation(b *strings.Builder, m entityModel, r relationModel) {
	relVar := m.Name + r.GoName + "Rel"
	joinFunc := "Join" + m.Name + r.GoName
	leftJoinFunc := "LeftJoin" + m.Name + r.GoName

	fmt.Fprintf(b, "// %s is the %q relation declared on %s, linking %s.%s to\n",
		relVar, r.Column, m.Name, m.Name, r.ParentKeyCol)
	fmt.Fprintf(b, "// %s.%s.\n", r.TargetType, r.ChildKeyCol)
	fmt.Fprintf(b, "var %s = orm.NewRelation[%s, %s](%q, %q, %s)\n\n",
		relVar, m.Name, r.TargetType, r.ParentKeyCol, r.ChildKeyCol, r.TargetTableVar)

	fmt.Fprintf(b, "// %s starts a typed INNER/LEFT JOIN from %s through the %q relation,\n",
		joinFunc, m.TableVar, r.Column)
	b.WriteString("// producing one orm.Row2 per matching row in a single round trip. See\n")
	b.WriteString("// orm.Join2's doc comment for why joinType == orm.LeftJoin is only safe\n")
	fmt.Fprintf(b, "// here when %s has no non-nullable columns that could legitimately come\n", r.TargetType)
	fmt.Fprintf(b, "// back NULL -- prefer %s for the general LEFT JOIN case.\n", leftJoinFunc)
	fmt.Fprintf(b, "func %s(left orm.Query[%s, *%s], joinType orm.JoinType) orm.Join2[%s, *%s, %s, *%s] {\n",
		joinFunc, m.Name, m.Name, m.Name, m.Name, r.TargetType, r.TargetType)
	fmt.Fprintf(b, "\treturn orm.JoinOn(left, %s, joinType)\n}\n\n", relVar)

	fmt.Fprintf(b, "// %s starts a null-safe LEFT JOIN from %s through the %q relation: an\n",
		leftJoinFunc, m.TableVar, r.Column)
	fmt.Fprintf(b, "// unmatched %s row comes back with an explicit orm.Option[%s] rather\n",
		m.Name, r.TargetType)
	b.WriteString("// than a zero-valued struct that could be mistaken for a real match.\n")
	fmt.Fprintf(b, "func %s(left orm.Query[%s, *%s]) orm.LeftJoin2[%s, *%s, %s, *%s] {\n",
		leftJoinFunc, m.Name, m.Name, m.Name, m.Name, r.TargetType, r.TargetType)
	fmt.Fprintf(b, "\treturn orm.LeftJoinOn(left, %s)\n}\n\n", relVar)
}
