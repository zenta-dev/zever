package resolver

import (
	"github.com/zenta-dev/zever/internal/dsl/ast"
	"github.com/zenta-dev/zever/internal/dsl/diag"
	"github.com/zenta-dev/zever/internal/dsl/ir"
)

// buildEntityIndex flattens every module's already-resolved entities into a
// single name -> *ir.Entity map. Names are globally unique across every
// declaration kind (Pass 0 guarantees this), so this is safe once Pass 1 has
// run.
func buildEntityIndex(modules []*ir.Module) map[string]*ir.Entity {
	idx := make(map[string]*ir.Entity)

	for _, m := range modules {
		for _, e := range m.Entities {
			idx[e.Name] = e
		}
	}

	return idx
}

// m2mEntry pairs a resolved many_to_many *ir.Relation with the *ir.Entity
// that owns it (an ir.Relation has no back-pointer to its owner), so the
// symmetry pass below can reason about both directions of a pair.
type m2mEntry struct {
	owner *ir.Entity
	rel   *ir.Relation
}

// resolveRelationsAndIndexes is Pass 2 (relations, plus the many-to-many
// symmetry pass) and Pass 3 (indexes). It requires entityByName to already
// hold every Pass-1-resolved *ir.Entity, since relation targets and index
// columns are resolved against it.
func resolveRelationsAndIndexes(decls []*ast.EntityDecl, entityByName map[string]*ir.Entity) diag.List {
	var diags diag.List

	var m2m []m2mEntry

	for _, decl := range decls {
		owner := entityByName[decl.Name]
		if owner == nil {
			// Unreachable given Pass 1's invariants; guarded defensively.
			continue
		}

		for _, rd := range decl.Relations {
			rel, d := resolveRelation(owner, rd, entityByName)
			diags = append(diags, d...)

			if rel == nil {
				continue
			}

			owner.Relations = append(owner.Relations, rel)

			if rel.Kind == ir.ManyToMany && rel.Target != nil && rel.JoinTable != "" {
				m2m = append(m2m, m2mEntry{owner: owner, rel: rel})
			}
		}

		for _, id := range decl.Indexes {
			index, d := resolveIndex(owner, id)
			diags = append(diags, d...)

			owner.Indexes = append(owner.Indexes, index)
		}
	}

	diags = append(diags, resolveManyToManySymmetry(m2m)...)

	return diags
}

// resolveRelation resolves one relation declaration. It returns a nil
// relation (and an ErrUnresolvedReference diagnostic) only when Target
// itself fails to resolve — every other problem (cross-module, bad
// foreign_key, bad on_delete, ...) is reported independently while still
// returning as complete a *ir.Relation as possible, same "collect everything"
// spirit as Pass 1.
func resolveRelation(owner *ir.Entity, rd *ast.RelationDecl, entityByName map[string]*ir.Entity) (*ir.Relation, diag.List) {
	var diags diag.List

	target, ok := entityByName[rd.Target]
	if !ok {
		return nil, diag.List{diag.Wrap("resolve", rd.TargetPos, ErrUnresolvedReference,
			"relation %s.%s references unknown entity %q", owner.Name, rd.FieldName, rd.Target)}
	}

	// Cross-module check is performed centrally in CheckCrossModule; not duplicated here.

	kind := toIRRelationKind(rd.Kind)

	rel := &ir.Relation{
		Kind:      kind,
		FieldName: rd.FieldName,
		Target:    target,
		Pos:       rd.Pos,
	}

	diags = append(diags, checkJoinBlockShape(owner, rd, kind)...)

	if kind == ir.ManyToMany {
		if rd.Join != nil {
			rel.JoinTable = rd.Join.Table
		}
	}

	fkName, fkPos, haveFK, d := resolveRelationAttributes(rd, rel)
	diags = append(diags, d...)

	if kind == ir.ManyToMany {
		if haveFK {
			diags = append(diags, diag.Wrap("resolve", fkPos, ErrInvalidRelation,
				"@foreign_key is not valid on many_to_many relations"))
		}

		return rel, diags
	}

	if !haveFK {
		diags = append(diags, diag.Wrap("resolve", rd.Pos, ErrInvalidRelation,
			"%s relation %s.%s requires @foreign_key(field)", relationKindLabel(kind), owner.Name, rd.FieldName))

		return rel, diags
	}

	field, ferr := resolveForeignKeyField(kind, owner, target, fkName, fkPos)
	if ferr != nil {
		diags = append(diags, ferr)
	} else {
		rel.ForeignKey = field
	}

	return rel, diags
}

// resolveForeignKeyField resolves the @foreign_key(field) name against the
// entity the relation kind dictates it must live on, additionally requiring
// @unique for has_one. It returns the resolved field on success and a nil
// diagnostic, or a nil field and a non-nil diagnostic on failure.
func resolveForeignKeyField(
	kind ir.RelationKind, owner, target *ir.Entity, fkName string, pos diag.Position,
) (*ir.Field, *diag.Diagnostic) {
	switch kind {
	case ir.HasMany:
		if f := target.FieldByName(fkName); f != nil {
			return f, nil
		}

		return nil, diag.Wrap("resolve", pos, ErrInvalidRelation,
			"has_many foreign key %q not found on target entity %s", fkName, target.Name)
	case ir.BelongsTo:
		if f := owner.FieldByName(fkName); f != nil {
			return f, nil
		}

		return nil, diag.Wrap("resolve", pos, ErrInvalidRelation,
			"belongs_to foreign key %q not found on owning entity %s", fkName, owner.Name)
	case ir.HasOne:
		f := target.FieldByName(fkName)
		if f == nil {
			return nil, diag.Wrap("resolve", pos, ErrInvalidRelation,
				"has_one foreign key %q not found on target entity %s", fkName, target.Name)
		}

		if !f.Unique {
			return nil, diag.Wrap("resolve", pos, ErrInvalidRelation,
				"has_one foreign key %q on target entity %s must be @unique", fkName, target.Name)
		}

		return f, nil
	case ir.ManyToMany:
		return nil, nil // unreachable: caller never invokes this for many_to_many
	default:
		return nil, nil // unreachable: toIRRelationKind only produces the four cases above
	}
}

// checkJoinBlockShape enforces "JoinBlock present iff Kind == ManyToMany".
func checkJoinBlockShape(owner *ir.Entity, rd *ast.RelationDecl, kind ir.RelationKind) diag.List {
	switch {
	case kind == ir.ManyToMany && rd.Join == nil:
		return diag.List{diag.Wrap("resolve", rd.Pos, ErrInvalidRelation,
			"many_to_many relation %s.%s requires a join_table block", owner.Name, rd.FieldName)}
	case kind != ir.ManyToMany && rd.Join != nil:
		return diag.List{diag.Wrap("resolve", rd.Join.Pos, ErrInvalidRelation,
			"join_table block is only valid on many_to_many relations")}
	default:
		return nil
	}
}

// resolveRelationAttributes decodes @foreign_key and @on_delete off a
// relation's attribute list, setting rel.OnDelete directly (foreign_key
// resolution needs the relation kind, done by the caller) and reporting any
// other attribute name as ErrInvalidRelation.
func resolveRelationAttributes(rd *ast.RelationDecl, rel *ir.Relation) (fkName string, fkPos diag.Position, haveFK bool, diags diag.List) {
	for _, attr := range rd.Attributes {
		switch attr.Name {
		case "foreign_key":
			name, d := decodeSingleIdentArg(attr, "foreign_key")
			if d != nil {
				diags = append(diags, d)
				continue
			}

			fkName, fkPos, haveFK = name, attr.Pos, true
		case "on_delete":
			resolveOnDeleteAttribute(attr, rel, &diags)
		default:
			diags = append(diags, diag.Wrap("resolve", attr.Pos, ErrInvalidRelation, "unknown relation attribute @%s", attr.Name))
		}
	}

	return fkName, fkPos, haveFK, diags
}

// resolveOnDeleteAttribute decodes one @on_delete(x) attribute, appending
// any diagnostic to diags and setting rel.OnDelete only on success.
func resolveOnDeleteAttribute(attr *ast.Attribute, rel *ir.Relation, diags *diag.List) {
	val, d := decodeSingleIdentArg(attr, "on_delete")
	if d != nil {
		*diags = append(*diags, d)
		return
	}

	switch val {
	case "restrict", "cascade", "set_null":
		rel.OnDelete = val
	default:
		*diags = append(*diags, diag.Wrap("resolve", attr.Pos, ErrInvalidRelation,
			"@on_delete value %q must be one of restrict, cascade, set_null", val))
	}
}

// toIRRelationKind maps the parser's RelationKind to the ir package's,
// spelled out explicitly (rather than relying on the two iota orders
// matching) so a future reordering of either enum fails loudly instead of
// silently mislabeling relations.
func toIRRelationKind(k ast.RelationKind) ir.RelationKind {
	switch k {
	case ast.HasMany:
		return ir.HasMany
	case ast.HasOne:
		return ir.HasOne
	case ast.BelongsTo:
		return ir.BelongsTo
	case ast.ManyToMany:
		return ir.ManyToMany
	default:
		return ir.HasMany // unreachable: the parser only emits the four kinds above
	}
}

// relationKindLabel renders an ir.RelationKind as its DSL keyword, for
// diagnostic messages.
func relationKindLabel(k ir.RelationKind) string {
	switch k {
	case ir.HasMany:
		return "has_many"
	case ir.HasOne:
		return "has_one"
	case ir.BelongsTo:
		return "belongs_to"
	case ir.ManyToMany:
		return "many_to_many"
	default:
		return "relation"
	}
}

// decodeSingleIdentArg decodes a single-argument attribute value accepted as
// either a bare identifier or a string literal (same shape as @schema).
func decodeSingleIdentArg(attr *ast.Attribute, name string) (string, *diag.Diagnostic) {
	if len(attr.Args) != 1 {
		return "", diag.Wrap("resolve", attr.Pos, ErrInvalidRelation, "@%s requires exactly one argument", name)
	}

	switch v := attr.Args[0].Value.(type) {
	case *ast.IdentValue:
		return v.Name, nil
	case *ast.StringLit:
		return v.Value, nil
	default:
		return "", diag.Wrap("resolve", attr.Pos, ErrInvalidRelation, "@%s argument must be an identifier or string", name)
	}
}

// resolveIndex resolves one index declaration's columns against the owning
// entity's own fields (never relations). Each bad column is reported
// independently — a bad second column never hides a bad first one — and the
// resulting Index only contains the columns that did resolve.
func resolveIndex(owner *ir.Entity, decl *ast.IndexDecl) (*ir.Index, diag.List) {
	var diags diag.List

	index := &ir.Index{Pos: decl.Pos}

	for i, col := range decl.Columns {
		f := owner.FieldByName(col)
		if f == nil {
			pos := decl.Pos
			if i < len(decl.ColumnPos) {
				pos = decl.ColumnPos[i]
			}

			diags = append(diags, diag.Wrap("resolve", pos, ErrUnresolvedReference,
				"index on entity %s references unknown column %q", owner.Name, col))

			continue
		}

		index.Columns = append(index.Columns, f)
	}

	for _, attr := range decl.Attributes {
		if attr.Name != "unique" {
			diags = append(diags, diag.Wrap("resolve", attr.Pos, ErrInvalidAttribute, "unknown index attribute @%s", attr.Name))
			continue
		}

		index.Unique = true
	}

	return index, diags
}

// resolveManyToManySymmetry matches every collected many_to_many relation
// with its reciprocal (same join table, owner/target swapped), linking
// Reciprocal both ways on a match and reporting ErrInvalidRelation on an
// unmatched one. It then separately checks for a join table name reused by
// a second, unrelated pair.
func resolveManyToManySymmetry(entries []m2mEntry) diag.List {
	var diags diag.List

	matched := make([]bool, len(entries))

	for i := range entries {
		if matched[i] {
			continue
		}

		e := entries[i]
		found := -1

		for j := i + 1; j < len(entries); j++ {
			if matched[j] {
				continue
			}

			o := entries[j]
			if o.rel.Target == e.owner && o.owner == e.rel.Target && o.rel.JoinTable == e.rel.JoinTable {
				found = j
				break
			}
		}

		if found < 0 {
			diags = append(diags, diag.Wrap("resolve", e.rel.Pos, ErrInvalidRelation,
				"many_to_many relation %s.%s (join_table %q) has no matching reciprocal many_to_many field on %s",
				e.owner.Name, e.rel.FieldName, e.rel.JoinTable, e.rel.Target.Name))

			continue
		}

		e.rel.Reciprocal = entries[found].rel
		entries[found].rel.Reciprocal = e.rel
		matched[i] = true
		matched[found] = true
	}

	diags = append(diags, checkJoinTableCollisions(entries)...)

	return diags
}

// checkJoinTableCollisions reports a join table name shared by more than one
// reciprocal pair — a real SQL-collision bug the future Atlas backend would
// otherwise hit at migration time instead of compile time.
func checkJoinTableCollisions(entries []m2mEntry) diag.List {
	var diags diag.List

	firstSeen := map[string]int{}

	for i, e := range entries {
		first, ok := firstSeen[e.rel.JoinTable]
		if !ok {
			firstSeen[e.rel.JoinTable] = i
			continue
		}

		if entries[first].rel.Reciprocal == e.rel || e.rel.Reciprocal == entries[first].rel {
			continue
		}

		diags = append(diags, diag.Wrap("resolve", e.rel.Pos, ErrInvalidRelation,
			"join_table %q is already used by a different many_to_many pair (%s.%s)",
			e.rel.JoinTable, entries[first].owner.Name, entries[first].rel.FieldName))
	}

	return diags
}
