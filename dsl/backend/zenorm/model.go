package zenorm

import (
	"fmt"
	"strings"

	"github.com/zenta-dev/zever/dsl/ir"
	"github.com/zenta-dev/zever/dsl/naming"
)

// initialisms keeps these words fully capitalised inside an identifier
// ("id" -> "ID", not "Id"), per Go style.
var initialisms = map[string]string{
	"api":   "API",
	"ascii": "ASCII",
	"cpu":   "CPU",
	"db":    "DB",
	"html":  "HTML",
	"http":  "HTTP",
	"https": "HTTPS",
	"id":    "ID",
	"ip":    "IP",
	"json":  "JSON",
	"sql":   "SQL",
	"ttl":   "TTL",
	"uri":   "URI",
	"url":   "URL",
	"uuid":  "UUID",
	"xml":   "XML",
}

// goName converts a schema identifier to an exported Go identifier,
// respecting Go initialisms: "id" -> "ID", "created_at" -> "CreatedAt".
func goName(s string) string {
	parts := strings.Split(naming.SnakeCase(s), "_")
	for i, p := range parts {
		if up, ok := initialisms[p]; ok {
			parts[i] = up

			continue
		}

		parts[i] = naming.PascalCase(p)
	}

	return strings.Join(parts, "")
}

// fieldModel is one scalar column of an entity.
type fieldModel struct {
	Column string // SQL column name
	GoName string // exported Go struct field name / <Entity>Cols field name
	GoType string // generated Go type (bare, never "*"-prefixed: a nullable
	// field's struct field type is orm.Option[GoType], never "*GoType")
	Import   string // standard-library import GoType needs, if any
	IsTime   bool   // GoType is time.Time
	Optional bool   // ir.Field.Optional: nullable column
}

// entityModel is everything renderEntityBody needs about one entity.
type entityModel struct {
	Name string // Pascal entity name, e.g. "User"
	Pkg  string // entity snake_case name, e.g. "user"; used in error tags

	Table    string // SQL table name, e.g. "users"
	TableVar string // generated orm.Table var name, e.g. "Users"
	ColsVar  string // generated orm.Column struct var name, e.g. "UserCols"

	Fields    []fieldModel
	PK        *fieldModel // nil when the entity declares no @primary field
	Relations []relationModel
}

// relationModel is one generated relation: enough to emit an
// orm.NewRelation var plus a typed Join<Entity><Relation>/
// LeftJoin<Entity><Relation> helper pair. The orm Join2/LeftJoin2 types do
// the actual join-column qualification and column-list rendering themselves
// (via Relation/Table), so this backend needs neither a TargetColumns nor
// a TargetScan string.
type relationModel struct {
	GoName string // exported Go name derived from the relation's field name, e.g. "Orders" or "User"
	Column string // schema field name, used in doc comments, e.g. "orders"

	TargetType     string // Pascal target entity name, e.g. "Order"
	TargetTableVar string // generated orm.Table var name for the target entity, e.g. "Orders"

	// ParentKeyCol/ChildKeyCol are the single-column equi-join's two sides:
	// "<owner>.ParentKeyCol = <target>.ChildKeyCol". See fillRelationKeys.
	ParentKeyCol string
	ChildKeyCol  string
}

// errOptionalPrimaryKey guards against a nullable primary key, which has no
// well-defined "the primary key of this row" (NullableColumn's own
// comparison methods deliberately take a bare V, not an Option[V]), so this
// backend rejects it outright rather than emitting a column no future
// mutation codegen could safely key off.
func errOptionalPrimaryKey(entityName, fieldName string) error {
	return fmt.Errorf("zenorm: entity %s: primary key field %q must not be optional (`?`)", entityName, fieldName)
}

func newEntityModel(e *ir.Entity) (entityModel, error) {
	snake := naming.SnakeCase(e.Name)

	m := entityModel{
		Name:  naming.PascalCase(e.Name),
		Pkg:   snake,
		Table: naming.PluralizeNaive(snake),
	}

	m.TableVar = naming.PluralizeNaive(naming.PascalCase(e.Name))
	m.ColsVar = m.Name + "Cols"

	for _, f := range e.Fields {
		goType, imp := goScalar(f.Type)

		fm := fieldModel{
			Column:   f.Name,
			GoName:   goName(f.Name),
			GoType:   goType,
			Import:   imp,
			IsTime:   f.Type.Scalar == ir.TTimestamp || f.Type.Scalar == ir.TDate,
			Optional: f.Optional,
		}

		m.Fields = append(m.Fields, fm)

		if f.Primary {
			if f.Optional {
				return entityModel{}, errOptionalPrimaryKey(e.Name, f.Name)
			}

			if m.PK == nil {
				pk := fm
				m.PK = &pk
			}
		}
	}

	for _, r := range e.Relations {
		if rm, ok := newRelationModel(e, r); ok {
			m.Relations = append(m.Relations, rm)
		}
	}

	return m, nil
}

// newRelationModel resolves one ir.Relation into the join keys the orm
// relation codegen needs, reporting false for relations this backend does
// not generate code for: many_to_many (a Join2/LeftJoin2 single-column
// equi-join has no join-table hop to express it), a relation whose target
// or foreign key the resolver could not supply, and a cross-module
// relation (unreachable today -- the resolver rejects those outright).
func newRelationModel(owner *ir.Entity, r *ir.Relation) (relationModel, bool) {
	if r.Kind == ir.ManyToMany || r.Target == nil || r.ForeignKey == nil {
		return relationModel{}, false
	}

	if owner.Module != r.Target.Module {
		return relationModel{}, false
	}

	rm := relationModel{
		GoName:     goName(r.FieldName),
		Column:     r.FieldName,
		TargetType: naming.PascalCase(r.Target.Name),
	}

	rm.TargetTableVar = naming.PluralizeNaive(rm.TargetType)

	if !fillRelationKeys(owner, r, &rm) {
		return relationModel{}, false
	}

	return rm, true
}

// fillRelationKeys sets the parent/child join columns from the relation's
// direction: for has_many/has_one the foreign key lives on the target and
// points at the owner's primary key; for belongs_to the foreign key lives
// on the owner and points at the target's primary key.
func fillRelationKeys(owner *ir.Entity, r *ir.Relation, rm *relationModel) bool {
	switch r.Kind {
	case ir.HasMany, ir.HasOne:
		pk := primaryField(owner)
		if pk == nil {
			return false
		}

		rm.ParentKeyCol = pk.Name
		rm.ChildKeyCol = r.ForeignKey.Name

		return true
	case ir.BelongsTo:
		pk := primaryField(r.Target)
		if pk == nil {
			return false
		}

		rm.ParentKeyCol = r.ForeignKey.Name
		rm.ChildKeyCol = pk.Name

		return true
	case ir.ManyToMany:
		return false
	default:
		return false
	}
}

func primaryField(e *ir.Entity) *ir.Field {
	if e == nil {
		return nil
	}

	for _, f := range e.Fields {
		if f.Primary {
			return f
		}
	}

	return nil
}

// goScalar maps a field's resolved type to its generated Go type and,
// when the mapping needs a standard-library import, that import path.
// A named enum reference maps to its PascalCase Go type; an inline
// enum(...) maps to string.
func goScalar(ft ir.FieldType) (goType, importPath string) {
	if ft.Scalar == ir.TEnum && ft.EnumName != "" {
		return naming.PascalCase(ft.EnumName), ""
	}

	switch ft.Scalar {
	case ir.TUUID, ir.TString:
		return "string", ""
	case ir.TInt32:
		return "int32", ""
	case ir.TInt64:
		return "int64", ""
	case ir.TFloat32:
		return "float32", ""
	case ir.TFloat64:
		return "float64", ""
	case ir.TBool:
		return "bool", ""
	case ir.TTimestamp, ir.TDate:
		return "time.Time", "time"
	case ir.TBytes:
		return "[]byte", ""
	case ir.TJSON:
		// orm.JSONText, not json.RawMessage: database/sql cannot scan
		// into RawMessage on recent Go toolchains, while JSONText binds
		// as TEXT, scans from TEXT, and marshals raw -- see orm/jsontext.go.
		return "orm.JSONText", ""
	case ir.TEnum:
		return "string", ""
	}

	return "any", ""
}
