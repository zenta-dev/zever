// Package render turns the orm's erased predicate Node/OrderTerm shapes into
// dialect-specific SQL text plus positional arguments.
//
// render is the single home for ALL of the orm's SQL text rendering, split
// by clause family across this package's files:
//
//   - render.go: predicate/expression rendering (renderExpr), plus plain
//     Select and Count.
//   - join.go: SelectJoin, a single projecting two-table INNER/LEFT JOIN.
//   - cte.go: SelectWith and CountWith, WITH [RECURSIVE] bodies.
//   - agg.go: GroupedSelect, GROUP BY/HAVING aggregate projection.
//   - window.go: WindowSelect, window-function projection.
//   - setop.go: SetOp, UNION/INTERSECT/EXCEPT composition.
//   - mutate.go: Insert/InsertMany/Update/Delete.
//   - upsert.go: the ON CONFLICT variants.
//   - json.go and fts.go: JSON-operator and full-text-search predicates.
//
// The public builders in the orm package convert their typed inputs into
// render's own erased shapes -- Node, OrderTerm, Assignment and the enum
// mirrors (Op, NodeKind, CompoundOp, JoinType, SetOpOp, JSONOp, FTSOp, ...).
// render never imports the orm root package (the orm root imports render),
// so each enum is a named int whose values match the builder's and is
// converted with a plain Go type conversion at the call site.
//
// Error-on-misuse contract: rendering never panics and never silently drops
// a clause. An unsupported or out-of-range NodeKind, an unknown Op, In/
// scalar-subquery shapes that project the wrong column count, or a
// malformed payload (e.g. a KindRaw node whose Value is not a RawExpr)
// surfaces as a rendering-time error, so the fluent API cannot emit
// silently-broken SQL. NFunc now renders the scalar-expression algebra (see
// renderFuncPredicate/renderScalar); NLit/NColumn/NUnary remain
// internal-only leaves of that algebra (NLit/NColumn are reachable only as
// expression arguments) and are not producible through the public API as
// standalone clauses.
package render

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/zenta-dev/zever/orm/dialect"
)

// Op mirrors the builder's Op values; render depends only on this package's own Op
// (never importing the orm root package -- the orm root imports render, not the other
// way around) so the two representations share the same underlying int
// values and ordering. Callers pass the builder's Op values converted with a plain
// Go type conversion (render.Op(n.Op)), legal because both are named `int`
// kinds with identical values.
type Op int

// Supported comparison operators; order and values match the builder's Op.
const (
	OpEq Op = iota
	OpNeq
	OpGt
	OpGte
	OpLt
	OpLte
	OpIn
	OpLike
	OpIsNull
	OpIsNotNull
	OpBetween
	// OpNotIn renders a KindIn node as `NOT IN (...)` instead of IN -- used
	// by the Column NotInSub builders. Renders identically
	// on every supported dialect.
	OpNotIn
	// OpEqAny/OpNeqAny/OpEqAll/OpNeqAll select the comparison and quantifier
	// of a KindArray node's Postgres array predicate (`= ANY`, `<> ANY`,
	// `= ALL`, `<> ALL`). Order and values match the builder's Op.
	OpEqAny
	OpNeqAny
	OpEqAll
	OpNeqAll
)

// NodeKind mirrors the builder's NodeKind values for the kinds this package
// handles.
type NodeKind int

// Supported node kinds; order and values match the corresponding subset of
// the builder's NodeKind.
const (
	KindNone NodeKind = iota
	KindLit
	KindColumn
	KindUnary
	KindBinary
	KindIn
	KindBetween
	KindLike
	KindCompound
	KindFunc
	KindSubquery
	KindRaw
	// KindJSON renders an NJSON predicate: the JSON expression over the
	// base column rendered per-dialect (jsonb operators on postgres, json1
	// functions on sqlite), followed by the comparison. See render/json.go.
	KindJSON
	// KindFTS renders an NFTS full-text-search expression over the base
	// column (tsvector machinery on postgres, FTS5 MATCH on sqlite). See
	// render/fts.go.
	KindFTS
	// KindTuple renders a multi-column row-value predicate -- a tuple of
	// outer columns compared against a subquery: `(a, b) [NOT] IN (SELECT
	// ...)` (Op In/NotIn) or `(a, b) OP (SELECT ...)` (Op Eq..Lte). Node's
	// Tuple field carries the LHS column list and Value the inner Subquery.
	// See renderTuple.
	KindTuple
	// KindArray renders an NArray Postgres array-quantifier predicate over
	// the base column: `col = ANY(ARRAY[..])`, `col <> ALL(ARRAY[..])`, etc.
	// Node.Value is the []any of bound elements and Node.Op selects both the
	// comparison and the ANY/ALL quantifier; the dialect must implement
	// dialect.ArrayDialect and report support, else rendering returns a typed
	// dialect.ErrUnsupportedByDialect. See render/array.go.
	KindArray
)

// CompoundOp mirrors the builder's CompoundOp values.
type CompoundOp int

// Supported compound operators; order and values match the builder's CompoundOp.
const (
	CompoundAnd CompoundOp = iota
	CompoundOr
	CompoundNot
)

// Node is render's own copy of the orm's erased predicate-tree shape. The
// builder's Node values are converted to this shape with a plain
// field-for-field literal at the call site (see the query builder), keeping
// render decoupled from the orm root package (no import cycle).
type Node struct {
	Kind     NodeKind
	Table    string
	Column   string
	Op       Op
	Value    any
	Compound CompoundOp
	Children []Node

	// Tuple is the LHS column list of a KindTuple row-value predicate (the
	// `(a, b)` of `(a, b) [NOT] IN (SELECT ...)` / `(a, b) OP (SELECT
	// ...)`), empty for every other kind. The columns are unqualified
	// codegen-derived names; the inner subquery travels in Value. See
	// renderTuple.
	Tuple []string

	// JSON is non-nil only when Kind == KindJSON; it carries the JSON
	// operator and path steps the base column (Table/Column) is operated
	// on. See render/json.go.
	JSON *JSONExpr

	// FTS is non-nil only when Kind == KindFTS; it carries the
	// full-text-search operator and search text the base column
	// (Table/Column) is operated on. See render/fts.go.
	FTS *FTSExpr

	// Func is non-nil only when Kind == KindFunc; it carries the scalar
	// function/CASE expression tree. At the top level the node's Op/Value
	// are the comparison applied to the expression's result; nested
	// occurrences are reached through Func. See render.go's renderFuncExpr.
	Func *FuncExpr
}

// FuncExpr mirrors the builder's FuncExpr: the erased scalar function/CASE expression
// tree an NFunc node (or an expression OrderTerm) carries. Args elements are
// themselves Nodes using the KindColumn (column leaf), KindLit (bound
// literal) or KindFunc (nested expression) kinds, so expressions nest to any
// depth. Case distinguishes a CASE expression (Whens/Else) from a plain
// named function call (Name/Args).
type FuncExpr struct {
	Name  string
	Args  []Node
	Case  bool
	Whens []FuncWhen
	Else  *Node
}

// FuncWhen mirrors the builder's FuncWhen: one CASE WHEN <Cond> THEN <Then> arm.
type FuncWhen struct {
	Cond Node
	Then Node
}

// JSONExpr mirrors the builder's JSONExpr: the erased JSON expression an NJSON node
// carries. Its Op is abstract (see render/json.go for how each op renders
// per dialect).
type JSONExpr struct {
	Op    JSONOp
	Steps []JSONStep
	Path  string
}

// JSONOp mirrors the builder's JSONOp values.
type JSONOp int

// Supported JSON operators; order and values match the builder's JSONOp.
const (
	JSONExtract JSONOp = iota
	JSONExtractText
	JSONPathExtract
	JSONPathExtractText
	JSONContains
	JSONKeyExists
	JSONType
	JSONArrayContains
	JSONLength
	JSONPathExists
	JSONPathMatch
	JSONPathExistsFunc
	JSONPathMatchFunc
	JSONPathQueryFirst
	JSONKeyExistsAny
	JSONKeyExistsAll
)

// JSONStep mirrors the builder's JSONStep: one step of a JSON path (an object key or
// an array index).
type JSONStep struct {
	Key     string
	Index   int64
	IsIndex bool
}

// FTSOp mirrors the builder's FTSOp values (same underlying int representation),
// so a caller-side `render.FTSOp(n.FTS.Op)` plain conversion is always
// legal.
type FTSOp int

// Supported full-text-search operators; order and values match the builder's FTSOp.
const (
	FTSMatch FTSOp = iota
	FTSMatchTSQuery
	FTSRank
)

// FTSMode is a retained mirror of the historical full-text mode enum (same
// underlying int representation and ordering). The postgres and sqlite
// renderers ignore it; it stays so erased shapes keep stable values.
type FTSMode int

// Retained full-text mode values; order and values are frozen for shape
// compatibility.
const (
	FTSNaturalLanguage FTSMode = iota
	FTSBoolean
	FTSQueryExpansion
)

// FTSExpr is the erased full-text-search expression an NFTS node (or FTS
// OrderTerm) carries. Its Op is abstract (see fts.go for how each op renders
// per dialect); Query is the search text, bound as a placeholder argument,
// never string-formatted. Mode and Columns are retained for shape
// compatibility and ignored by the postgres/sqlite renderers.
type FTSExpr struct {
	Op      FTSOp
	Query   string
	Mode    FTSMode
	Columns []string
}

// RawExpr is the erased payload of a KindRaw node, produced by
// the UnsafeRaw escape hatch: a caller-supplied SQL fragment plus positional bound
// arguments. The fragment is rendered verbatim (a security escape hatch
// the caller must audit -- see the the unsafe-SQL linter); the args
// are ALWAYS placeholder-bound, never string-formatted, so a bound value
// containing `'`, `;` or a NUL byte alters only its own value, never the
// SQL structure.
type RawExpr struct {
	Fragment string
	Args     []any
}

// Subquery is the erased inner SELECT carried by a subquery predicate: a
// KindIn node whose Value is a Subquery (`col [NOT] IN (SELECT ...)`), a
// KindBinary comparison whose Value is a Subquery (`col OP (SELECT ...)`),
// or a KindSubquery node (`EXISTS (SELECT ...)`). It is built by the query builder from
// an inner Query at predicate-CONSTRUCTION time -- never from pre-rendered
// SQL text -- so its clauses render at execution time through the same
// dialect and the same placeholder counter as the enclosing statement.
//
// Columns is the inner query's projection; the In-subquery and scalar-
// comparison consumers require exactly one column (rendering errors
// otherwise), while EXISTS accepts any projection.
type Subquery struct {
	Table   string
	Columns []string
	Where   Node
	Order   []OrderTerm
	Limit   int
	Offset  int
}

// OuterRef is the erased marker for a correlated reference to a column of
// the ENCLOSING query, carried as a KindBinary node's Value in place of a
// bound value (built by the Outer/OuterNullable helpers in an inner subquery's
// Where). Table is the referenced column's real home table and Column its
// name; the SQL text is NOT produced here -- the renderer validates Table
// against the ACTUAL enclosing statement's table set from the rendering
// scope at execution time and then qualifies the column with it (see
// renderScope and renderOuterRef), so a construction-time snapshot can never
// freeze the wrong outer table.
type OuterRef struct {
	Table  string
	Column string
}

// renderScope carries the per-statement table facts render's subquery chain
// needs to resolve a correlated OuterRef marker into fully qualified SQL
// text at execution time. Each fact is a SET of tables, because a statement
// whose FROM is multi-table (a Join2/Join3) can be the enclosing query of a
// nested subquery:
//
//	stmt -- the tables of the statement being rendered, handed to any
//	        subquery nested in its WHERE as THAT subquery's correlation
//	        target (the "enclosing query" one level down). A plain
//	        single-table SELECT contributes exactly one table; a join
//	        contributes every table it FROMs.
//	corr -- the tables a correlated marker in THIS statement's WHERE
//	        resolves against: the immediately enclosing statement's tables,
//	        or nil when this statement is not itself nested inside a
//	        subquery (no outer query exists, so a marker is a misuse and
//	        must fail closed).
//
// A marker therefore binds to its DIRECTLY enclosing SELECT's table set --
// the classic `NOT EXISTS (... inner.col = outer.col ...)` level, including
// the multi-table set a join contributes -- and one further level of nesting
// binds the deepest marker to the middle subquery's own table. Only the
// plain single-table SELECT/COUNT chain and the projecting join renderers
// set stmt; the remaining multi-table statement shapes (setops, aggregates,
// CTE bodies, mutations) render their WHERE through the zero scope, so a
// marker can never bind within them.
type renderScope struct {
	stmt []string
	corr []string
}

// scopeHas reports whether tables contains name.
func scopeHas(tables []string, name string) bool {
	for _, t := range tables {
		if t == name {
			return true
		}
	}

	return false
}

// quoteTables renders a table-name set for an error message, each name
// double-quoted like QuoteIdent would render it.
func quoteTables(tables []string) string {
	quoted := make([]string, len(tables))
	for i, t := range tables {
		quoted[i] = strconv.Quote(t)
	}

	return strings.Join(quoted, ", ")
}

// NullsOrder mirrors the builder's NullsOrder values (same underlying int
// representation), so a caller-side `render.NullsOrder(o.Nulls)` plain
// conversion is always legal. NullsDefault emits no NULLS suffix; NullsFirst
// appends `NULLS FIRST`, NullsLast `NULLS LAST`.
type NullsOrder int

// Supported NULLS-ordering positions. NullsDefault is the zero value: no
// modifier, exactly the pre-NULLS render.
const (
	NullsDefault NullsOrder = iota
	NullsFirst
	NullsLast
)

// OrderTerm is one ORDER BY column, plus its direction. FTS is non-nil only
// for a term ordering by an FTS ranking expression (Postgres ts_rank)
// instead of a plain quoted column -- see render/fts.go. Func is non-nil for
// a term ordering by a scalar expression (COALESCE/LOWER/CASE), whose
// bound arguments are collected in text order alongside FTS's -- see
// joinOrderBy. Nulls is the optional NULLS FIRST/LAST suffix, gated by
// dialect.NullsOrderDialect in joinOrderBy.
type OrderTerm struct {
	Column string
	// Table is the base column's home table, used to qualify a term's column
	// reference in multi-table statements. Empty when unknown.
	Table string
	Desc  bool
	Nulls NullsOrder
	FTS   *FTSExpr
	Func  *FuncExpr
}

// LockMode identifies a row-level locking clause on a SELECT. Its values
// mirror the builder's LockMode (same underlying int representation), so callers
// convert with a plain Go type conversion.
type LockMode int

// Supported row-lock modes. LockNone renders no locking clause.
const (
	LockNone LockMode = iota
	LockForUpdate
	LockForShare
	// LockForNoKeyUpdate and LockForKeyShare are the two weaker Postgres row
	// lock strengths (`FOR NO KEY UPDATE`, `FOR KEY SHARE`). They are
	// Postgres-only; the orm layer gates them via ExtendedLockingDialect.
	LockForNoKeyUpdate
	LockForKeyShare
)

// SelectModifiers carries the SELECT-clause modifiers beyond the historical
// table/columns/where/order/limit/offset set: DISTINCT / DISTINCT ON, an
// optional row-level lock (with its OF list and NOWAIT/SKIP LOCKED suffix),
// and an optional TABLESAMPLE clause. The zero value renders exactly the
// pre-modifier SQL.
//
// NoWait/SkipLocked are only meaningful with a non-zero Lock; the renderer
// rejects the DISTINCT + Lock combination (forbidden by the SQL standard and
// by Postgres) rather than emitting invalid SQL.
type SelectModifiers struct {
	Distinct bool
	// DistinctOn is the Postgres `DISTINCT ON (cols)` list. A nil slice means
	// the modifier is unset; an explicitly empty slice is a caller error (see
	// validateSelectModifiers). DistinctOn supersedes Distinct in the
	// rendered prefix.
	DistinctOn []string
	Lock       LockMode
	// LockOf is the optional Postgres `OF <table>, ...` list appended to the
	// lock clause. Postgres's OF list is table names (not column names); the builder
	// builds it from each projected column's home table. Empty renders no OF.
	LockOf     []string
	NoWait     bool
	SkipLocked bool
	// Tablesample is the optional Postgres `TABLESAMPLE <method> (<arg>)`
	// clause. The zero value (empty Method) renders no sampling. Method is
	// validated against a fixed allowlist by validateSelectModifiers.
	Tablesample Tablesample
}

// Tablesample is the Postgres `TABLESAMPLE <method> (<arg>)` clause. Method
// is one of the allowlisted sampling methods (SYSTEM, BERNOULLI); Arg is the
// percentage argument, rendered as an unquoted numeric literal. The zero
// value renders no clause.
type Tablesample struct {
	Method string
	Arg    float64
}

// validateSelectModifiers rejects the invalid modifier combinations and
// out-of-range lock modes every SELECT render path must fail closed on --
// DISTINCT/DISTINCT ON + row lock, a lock modifier with no lock mode, an
// empty DISTINCT ON list, and a DISTINCT ON request on a dialect that lacks
// the capability. It is shared by the plain SELECT renderer and the projected
// SELECT renderer.
func validateSelectModifiers(d dialect.Dialect, mods SelectModifiers) error {
	// DISTINCT and a row lock are mutually exclusive in standard SQL and on
	// Postgres; fail closed rather than emit a statement the server rejects.
	if (mods.Distinct || mods.DistinctOn != nil) && mods.Lock != LockNone {
		return errors.New("orm/render: DISTINCT cannot be combined with a row lock (FOR UPDATE/FOR SHARE)")
	}

	if mods.DistinctOn != nil {
		if len(mods.DistinctOn) == 0 {
			return errors.New("orm/render: DISTINCT ON requires at least one column")
		}

		if !supportsDistinctOn(d) {
			return fmt.Errorf("orm/render: %w: dialect %q does not support DISTINCT ON", dialect.ErrUnsupportedByDialect, d.Name())
		}
	}

	if err := validateTablesample(d, mods.Tablesample); err != nil {
		return err
	}

	// A lock modifier with no lock mode, or an unknown lock mode, must never
	// be silently dropped.
	switch mods.Lock { //nolint:exhaustive // the default branch rejects unknown modes
	case LockNone:
		if mods.NoWait || mods.SkipLocked {
			return errors.New("orm/render: NOWAIT/SKIP LOCKED require a row lock (FOR UPDATE/FOR SHARE)")
		}

		if len(mods.LockOf) > 0 {
			return errors.New("orm/render: FOR UPDATE OF requires a row lock mode")
		}
	case LockForUpdate, LockForShare, LockForNoKeyUpdate, LockForKeyShare:
	default:
		return fmt.Errorf("orm/render: unknown lock mode %d", mods.Lock)
	}

	return nil
}

// supportsDistinctOn reports whether d implements dialect.DistinctOnDialect
// and reports support. A dialect that does not implement the interface is
// treated as unsupported, matching the repo's capability-gate convention.
func supportsDistinctOn(d dialect.Dialect) bool {
	dd, ok := d.(dialect.DistinctOnDialect)

	return ok && dd.SupportsDistinctOn()
}

// tablesampleMethods is the allowlist of sampling methods render will emit. The
// method is interpolated into the rendered SQL, so it MUST come from this
// fixed set -- never caller-controlled text -- exactly like an identifier.
var tablesampleMethods = map[string]struct{}{
	"SYSTEM":    {},
	"BERNOULLI": {},
}

// validateTablesample rejects an unknown sampling method, an out-of-range
// percentage, and a TABLESAMPLE request on a dialect that lacks the
// capability -- all as typed rendering-time errors, never silently-dropped
// SQL. The zero value is valid and renders no clause.
func validateTablesample(d dialect.Dialect, ts Tablesample) error {
	if ts.Method == "" {
		return nil
	}

	method := strings.ToUpper(ts.Method)

	if _, ok := tablesampleMethods[method]; !ok {
		return fmt.Errorf("orm/render: TABLESAMPLE method %q is not supported (use SYSTEM or BERNOULLI)", ts.Method)
	}

	if ts.Arg < 0 || ts.Arg > 100 {
		return fmt.Errorf("orm/render: TABLESAMPLE percentage %v is out of range [0, 100]", ts.Arg)
	}

	td, ok := d.(dialect.TablesampleDialect)
	if !ok || !td.SupportsTablesample() {
		return fmt.Errorf("orm/render: %w: dialect %q does not support TABLESAMPLE", dialect.ErrUnsupportedByDialect, d.Name())
	}

	return nil
}

// tablesampleClause renders the validated TABLESAMPLE clause, or "" when no
// sampling is set. The method is normalized to upper case and the percentage
// rendered as an unquoted numeric literal.
func tablesampleClause(ts Tablesample) string {
	if ts.Method == "" {
		return ""
	}

	return " TABLESAMPLE " + strings.ToUpper(ts.Method) + " (" + strconv.FormatFloat(ts.Arg, 'f', -1, 64) + ")"
}

// lockClause renders the trailing row-lock clause for mods, or "" when no
// lock is set. It emits the lock strength keyword, the optional Postgres
// `OF <tables>` list, then NOWAIT or SKIP LOCKED (mutually exclusive in the
// public API -- last one set wins -- so at most one is ever emitted).
func lockClause(d dialect.Dialect, mods SelectModifiers) string {
	var base string

	switch mods.Lock { //nolint:exhaustive // LockNone and out-of-range values render no clause
	case LockForUpdate:
		base = "FOR UPDATE"
	case LockForShare:
		base = "FOR SHARE"
	case LockForNoKeyUpdate:
		base = "FOR NO KEY UPDATE"
	case LockForKeyShare:
		base = "FOR KEY SHARE"
	default:
		return ""
	}

	out := " " + base

	if len(mods.LockOf) > 0 {
		out += " OF " + joinQuoted(d, mods.LockOf)
	}

	if mods.NoWait {
		out += " NOWAIT"
	} else if mods.SkipLocked {
		out += " SKIP LOCKED"
	}

	return out
}

// argCounter tracks how many positional args have been emitted so far, so
// placeholders can be numbered correctly for dialects like Postgres.
type argCounter struct{ n int }

func (c *argCounter) next() int {
	c.n++

	return c.n
}

// renderSelectText renders a SELECT statement (no joins) and its positional
// arguments for dialect. It is the cache-miss body behind Select (see
// shapecache.go), which caches the rendered text under the shape's
// fingerprint. Column names are emitted unqualified. err is non-nil when
// where contains an unsupported/unknown NodeKind or an unknown Op -- a
// rendering-time error, never silently-dropped SQL.
func renderSelectText(
	d dialect.Dialect,
	table string,
	columns []string,
	where Node,
	order []OrderTerm,
	limit, offset int,
	mods SelectModifiers,
) (query string, args []any, err error) {
	// DISTINCT and a row lock are mutually exclusive in standard SQL and on
	// Postgres; fail closed rather than emit a statement the server rejects.
	if err := validateSelectModifiers(d, mods); err != nil {
		return "", nil, err
	}

	// A top-level SELECT is itself the outermost statement: its own WHERE
	// carries no correlation target, and any subquery nested in that WHERE
	// correlates against table (this statement becomes the enclosing query
	// one level down).
	return renderSelectInto(d, table, columns, where, order, limit, offset, &argCounter{}, renderScope{stmt: []string{table}}, mods)
}

// renderSelectInto renders a SELECT statement's text and positional
// arguments onto the FRESH counter shared by the sibling clause renders in
// the enclosing statement. Every other renderer starts from a zeroed
// &argCounter{} (see renderSelectText); a subquery instead continues the
// ENCLOSING expression's counter so Postgres-style numbered placeholders
// stay sequential across the whole statement text. All placeholders are
// emitted and their bound arguments appended in text order, so the returned
// args line up positionally with the emitted placeholders.
//
// scope carries the correlation facts of the statement being rendered (see
// renderScope): this statement's own WHERE resolves correlated markers
// against scope.corr, and any subquery nested in that WHERE correlates
// against scope.stmt -- this statement's table.
func renderSelectInto(
	d dialect.Dialect,
	table string,
	columns []string,
	where Node,
	order []OrderTerm,
	limit, offset int,
	counter *argCounter,
	scope renderScope,
	mods SelectModifiers,
) (query string, args []any, err error) {
	var b strings.Builder

	writeSelectPrefix(&b, d, mods)

	b.WriteString(joinQuoted(d, columns))
	b.WriteString(" FROM ")
	b.WriteString(d.QuoteIdent(table))
	b.WriteString(tablesampleClause(mods.Tablesample))

	clause, whereArgs, err := renderExprCorr(d, where, counter, scope)
	if err != nil {
		return "", nil, err
	}

	if clause != "" {
		b.WriteString(" WHERE ")
		b.WriteString(clause)
	}

	args = append(args, whereArgs...)

	if len(order) > 0 {
		b.WriteString(" ORDER BY ")

		orderText, orderArgs, err := joinOrderBy(d, order, counter)
		if err != nil {
			return "", nil, err
		}

		b.WriteString(orderText)

		args = append(args, orderArgs...)
	}

	if limit > 0 {
		b.WriteString(" LIMIT ")
		b.WriteString(d.Placeholder(counter.next()))

		args = append(args, limit)
	}

	if offset > 0 {
		b.WriteString(" OFFSET ")
		b.WriteString(d.Placeholder(counter.next()))

		args = append(args, offset)
	}

	b.WriteString(lockClause(d, mods))

	return b.String(), args, nil
}

// renderCountText renders a `SELECT COUNT(*) FROM table WHERE ...`
// statement (order/limit/offset are meaningless for a count and are never
// rendered). It is the cache-miss body behind Count (see shapecache.go).
// err is non-nil under the same conditions renderSelectText's is. COUNT is
// itself a single-table statement, so its WHERE carries no correlation
// target and any subquery nested in it correlates against table.
func renderCountText(d dialect.Dialect, table string, where Node) (query string, args []any, err error) {
	var b strings.Builder

	b.WriteString("SELECT COUNT(*) FROM ")
	b.WriteString(d.QuoteIdent(table))

	counter := &argCounter{}

	clause, whereArgs, err := renderExprCorr(d, where, counter, renderScope{stmt: []string{table}})
	if err != nil {
		return "", nil, err
	}

	if clause != "" {
		b.WriteString(" WHERE ")
		b.WriteString(clause)
	}

	return b.String(), whereArgs, nil
}

// writeSelectPrefix writes the SELECT keyword plus the DISTINCT / DISTINCT ON
// prefix (whichever the modifiers call for) into b. It is shared by the plain
// and projected SELECT renderers so the two prefixes can never drift.
func writeSelectPrefix(b *strings.Builder, d dialect.Dialect, mods SelectModifiers) {
	switch {
	case len(mods.DistinctOn) > 0:
		b.WriteString("SELECT DISTINCT ON (")
		b.WriteString(joinQuoted(d, mods.DistinctOn))
		b.WriteString(") ")
	case mods.Distinct:
		b.WriteString("SELECT DISTINCT ")
	default:
		b.WriteString("SELECT ")
	}
}

func joinQuoted(d dialect.Dialect, columns []string) string {
	quoted := make([]string, len(columns))
	for i, c := range columns {
		quoted[i] = quoteColumn(d, c)
	}

	return strings.Join(quoted, ", ")
}

// joinOrderBy renders the ORDER BY term list, continuing counter for any
// FTS ranking or scalar-expression terms' placeholders and returning their
// bound arguments in term order (a plain column term binds nothing). A term
// with Func set renders as its scalar expression (e.g.
// `COALESCE("bio", ?) ASC`); err is non-nil when such an expression is
// malformed (see renderFuncExpr). A term with a non-default Nulls renders an
// `NULLS FIRST`/`NULLS LAST` suffix after its direction; the feature is
// gated at this single choke point so no ORDER BY render path can silently
// drop it -- an unsupported dialect yields the typed
// dialect.ErrUnsupportedByDialect.
func joinOrderBy(d dialect.Dialect, order []OrderTerm, counter *argCounter) (string, []any, error) {
	parts := make([]string, len(order))

	var args []any

	for i, o := range order {
		var part string

		switch {
		case o.Func != nil:
			text, fnArgs, err := renderFuncExpr(d, o.Func, counter, renderScope{})
			if err != nil {
				return "", nil, err
			}

			part = text

			args = append(args, fnArgs...)
		case o.FTS != nil:
			text, ftsArgs := ftsOrderExpr(d, o, counter)

			part = text

			args = append(args, ftsArgs...)
		default:
			part = quoteColumn(d, o.Column)
		}

		if o.Desc {
			part += " DESC"
		} else {
			part += " ASC"
		}

		if o.Nulls != NullsDefault {
			if !supportsNullsOrdering(d) {
				return "", nil, fmt.Errorf("orm/render: %w: dialect %q does not support NULLS FIRST/LAST", dialect.ErrUnsupportedByDialect, d.Name())
			}

			switch o.Nulls { //nolint:exhaustive // NullsDefault handled above; the default rejects out-of-range values
			case NullsFirst:
				part += " NULLS FIRST"
			case NullsLast:
				part += " NULLS LAST"
			default:
				return "", nil, fmt.Errorf("orm/render: unknown nulls order %d", o.Nulls)
			}
		}

		parts[i] = part
	}

	return strings.Join(parts, ", "), args, nil
}

// supportsNullsOrdering reports whether d implements
// dialect.NullsOrderDialect and reports support. A dialect that does not
// implement the interface at all (e.g. a base-only dialect) is treated as
// unsupported, matching the repo's capability-gate convention.
func supportsNullsOrdering(d dialect.Dialect) bool {
	nd, ok := d.(dialect.NullsOrderDialect)

	return ok && nd.SupportsNullsOrdering()
}

// quoteColumn quotes col, splitting a "table.column" qualifier if col
// carries one.
func quoteColumn(d dialect.Dialect, col string) string {
	if table, column, ok := strings.Cut(col, "."); ok {
		return d.QuoteIdent(table) + "." + d.QuoteIdent(column)
	}

	return d.QuoteIdent(col)
}

// renderExpr renders n's clause text AND collects its bound arguments in
// one traversal (unlike orm/engine's split text/args passes, this package
// has no shared query-shape cache to keep separate, so a single combined
// walk is simpler and has no correctness downside). err is non-nil for an
// unsupported/unknown NodeKind, an unknown Op, or a malformed KindRaw
// payload -- the fluent API must never emit silently-broken SQL, so
// misuse surfaces as a rendering-time error rather than a dropped clause.
//
// renderExpr is the ZERO-scope wrapper: marker-free (non-correlated)
// statements only. The remaining multi-table statement shapes (setops,
// aggregates, CTE bodies, mutations) that render their WHERE here cannot
// bind a correlated reference at all -- a misused marker inside such a
// statement fails here as a no-enclosing-query error. The single-table
// SELECT/COUNT chain and the projecting join renderers render through
// renderExprCorr instead, so their subqueries can correlate against the
// enclosing statement's tables. Tests that exercise a specific expression
// may also call renderExpr directly for convenience.
func renderExpr(d dialect.Dialect, n Node, counter *argCounter) (clause string, args []any, err error) {
	return renderExprCorr(d, n, counter, renderScope{})
}

// renderExprCorr is renderExpr with correlation: scope names the enclosing
// statement's table facts (see renderScope) so a correlated OuterRef marker
// resolves to the SQL of the DIRECTLY enclosing query. The zero scope
// renders exactly the same clauses renderExpr always has; a marker in that
// scope is a misuse and errors out.
func renderExprCorr(d dialect.Dialect, n Node, counter *argCounter, scope renderScope) (clause string, args []any, err error) {
	switch n.Kind { //nolint:exhaustive // reserved/unsupported kinds rejected by default
	case KindNone:
		return "", nil, nil
	case KindBinary:
		return renderBinary(d, n, counter, scope)
	case KindIn:
		return renderIn(d, n, counter, scope)
	case KindBetween:
		clause, args = renderBetween(d, n, counter)
	case KindLike:
		clause, args = renderLike(d, n, counter)
	case KindCompound:
		return renderCompound(d, n, counter, scope)
	case KindJSON:
		return renderJSON(d, n, counter)
	case KindFTS:
		return renderFTS(d, n, counter)
	case KindRaw:
		return renderRaw(d, n, counter)
	case KindSubquery:
		return renderExists(d, n, counter, scope)
	case KindTuple:
		return renderTuple(d, n, counter, scope)
	case KindArray:
		return renderArray(d, n, counter)
	case KindFunc:
		return renderFuncPredicate(d, n, counter, scope)
	default:
		// KindLit/KindColumn/KindUnary are reserved and unproducible
		// through the public API as standalone predicates today -- or the
		// kind is entirely out of range; both are caller bugs that must not
		// be silently swallowed into empty SQL text. (KindLit/KindColumn
		// do occur as scalar-expression ARGUMENTS, but those are reached
		// through renderScalar, never as a top-level clause.)
		return "", nil, fmt.Errorf("orm/render: unsupported node kind %d", n.Kind)
	}

	return clause, args, nil
}

// renderRaw renders a KindRaw node: its RawExpr fragment verbatim, with
// every `?` marker rewritten to the dialect's numbered placeholder (so a
// Postgres-shaped dialect gets $1, $2, ... in clause order, consistent with
// surrounding clauses' argCounter numbering), and the fragment's args bound
// positionally. A fragment with no `?` marker binds nothing.
//
// The args are bound as driver arguments, NEVER formatted into the SQL
// text, so a bound value can never alter the SQL structure -- the fragment
// itself is the caller-audited escape hatch (the UnsafeRaw escape hatch is flagged by
// the the unsafe-SQL linter). A `?` marker with no corresponding arg
// position is still emitted: the placeholder/arg mismatch surfaces as a
// driver error at execution rather than a panic or a dropped placeholder,
// so renderRaw fails closed.
//
// err is non-nil when n.Value is not a RawExpr -- a malformed raw node
// renders nothing usable and must not silently become an empty clause.
func renderRaw(d dialect.Dialect, n Node, counter *argCounter) (clause string, args []any, err error) {
	re, ok := n.Value.(RawExpr)
	if !ok {
		return "", nil, fmt.Errorf("orm/render: raw node value of type %T is not a RawExpr", n.Value)
	}

	if !strings.Contains(re.Fragment, "?") {
		return re.Fragment, nil, nil
	}

	var b strings.Builder

	for _, r := range re.Fragment {
		if r != '?' {
			b.WriteRune(r)

			continue
		}

		b.WriteString(d.Placeholder(counter.next()))

		if len(args) < len(re.Args) {
			args = append(args, re.Args[len(args)])
		}
	}

	return b.String(), args, nil
}

func renderBinary(d dialect.Dialect, n Node, counter *argCounter, scope renderScope) (clause string, args []any, err error) {
	col := quoteColumn(d, n.Column)

	switch n.Op { //nolint:exhaustive // only the binary comparison ops are legal here; the rest error in default
	case OpIsNull:
		return col + " IS NULL", nil, nil
	case OpIsNotNull:
		return col + " IS NOT NULL", nil, nil
	case OpIn, OpLike, OpBetween:
		// Never reached: In/Like/Between build KindIn/KindLike/KindBetween
		// nodes, handled by their own render funcs before renderBinary is
		// called.
		return "", nil, fmt.Errorf("orm/render: operator %d is not a binary comparison", n.Op)
	default:
		symbol, err := opSymbol(n.Op)
		if err != nil {
			return "", nil, err
		}

		if sq, ok := n.Value.(Subquery); ok {
			// A scalar comparison against a subquery: `col OP (SELECT ...)`.
			// The subquery must return exactly one cell, so a multi-column
			// projection is a caller bug surfaced here rather than three
			// columns of broken SQL.
			if len(sq.Columns) != 1 {
				return "", nil, fmt.Errorf("orm/render: scalar subquery must project exactly one column, got %d (project one via the query's Columns selector)", len(sq.Columns))
			}

			innerSQL, innerArgs, err := renderSubquery(d, sq, counter, scope)
			if err != nil {
				return "", nil, err
			}

			return col + " " + symbol + " (" + innerSQL + ")", innerArgs, nil
		}

		if ref, ok := n.Value.(OuterRef); ok {
			// A correlated column-to-column comparison against the enclosing
			// query, built by the Outer/OuterNullable helpers: render the outer
			// column fully qualified (outer.table.column), its table name
			// resolved from the rendering scope at execution time, and bind
			// NO argument -- the reference is a column, not a value, so it
			// must never occupy a placeholder slot (which would silently
			// corrupt $N numbering for the sibling bound args).
			return renderOuterRef(d, col, symbol, ref, scope)
		}

		ph := d.Placeholder(counter.next())

		return col + " " + symbol + " " + ph, []any{n.Value}, nil
	}
}

// renderOuterRef resolves a correlated OuterRef marker against the
// enclosing statement named by scope and renders `lhs OP "outer.table.col"`,
// binding no argument. The referenced column's own home table (ref.Table)
// qualifies the rendered column, so it is never confused with the inner
// statement's own unqualified columns.
//
// err is non-nil when the marker cannot legally bind:
//
//   - no enclosing query exists (scope.corr empty -- the marker rode a
//     statement shape that renders through the zero scope, e.g. the
//     outermost SELECT/JOIN itself or a mutation), or
//   - the marker's stored table is not one of the enclosing statement's
//     tables (scope.corr -- a reference to a table outside the join set or
//     across a non-adjacent outer query), or
//   - the marker's table is ALSO the current statement's own FROM table
//     (scope.stmt -- the inner query shadows the outer name, so the
//     qualified reference would silently bind inside and never reach the
//     outer row).
//
// All are typed rendering-time errors, never silently-wrong SQL.
func renderOuterRef(d dialect.Dialect, lhs, symbol string, ref OuterRef, scope renderScope) (string, []any, error) {
	if len(scope.corr) == 0 {
		return "", nil, fmt.Errorf("orm/render: correlated reference to %q.%q "+
			"has no enclosing query to bind to "+
			"(the Outer/OuterNullable helpers are only valid inside a subquery predicate's Where, "+
			"correlated against the directly enclosing SELECT's tables)", ref.Table, ref.Column)
	}

	if !scopeHas(scope.corr, ref.Table) {
		if len(scope.corr) == 1 {
			return "", nil, fmt.Errorf("orm/render: correlated reference to %q.%q "+
				"does not match the enclosing query's table %q "+
				"(correlation is limited to the directly enclosing statement's tables)",
				ref.Table, ref.Column, scope.corr[0])
		}

		return "", nil, fmt.Errorf("orm/render: correlated reference to %q.%q "+
			"does not match any of the enclosing query's tables (%s) "+
			"(correlation is limited to the directly enclosing statement's tables)",
			ref.Table, ref.Column, quoteTables(scope.corr))
	}

	if scopeHas(scope.stmt, ref.Table) {
		return "", nil, fmt.Errorf("orm/render: correlated reference to %q.%q "+
			"is shadowed by the subquery's own FROM table %q (rename or alias one of them; "+
			"the qualified reference would otherwise bind inside the subquery, not the enclosing query)",
			ref.Table, ref.Column, ref.Table)
	}

	return lhs + " " + symbol + " " + quoteColumn(d, ref.Table+"."+ref.Column), nil, nil
}

func renderLike(d dialect.Dialect, n Node, counter *argCounter) (clause string, args []any) {
	col := quoteColumn(d, n.Column)
	ph := d.Placeholder(counter.next())

	// ESCAPE '\' is always appended: it works identically on
	// Postgres/SQLite, and SQLite requires an explicit ESCAPE clause
	// to get any escaping at all. the Contains/StartsWith/EndsWith builders already
	// escape \, % and _ in their bound argument, so this clause makes that
	// escaping meaningful; the LikeRaw builder deliberately skips that escaping but
	// still gets the same ESCAPE clause here for consistent syntax.
	return col + " LIKE " + ph + " ESCAPE '\\'", []any{n.Value}
}

// renderBetween renders a `col BETWEEN ph1 AND ph2` clause. value is always
// a [2]any leaf, produced by Column.Between/NullableColumn.Between.
func renderBetween(d dialect.Dialect, n Node, counter *argCounter) (clause string, args []any) {
	col := quoteColumn(d, n.Column)

	lo, hi := any(nil), any(nil)
	if pair, ok := n.Value.([2]any); ok {
		lo, hi = pair[0], pair[1]
	}

	ph1 := d.Placeholder(counter.next())
	ph2 := d.Placeholder(counter.next())

	return col + " BETWEEN " + ph1 + " AND " + ph2, []any{lo, hi}
}

// renderIn renders an IN (...) clause with one placeholder per element, or
// -- when the value is a Subquery -- a `col [NOT] IN (SELECT ...)` clause
// rendering the inner query inline (continuing counter and the enclosing
// scope, so a correlated WHERE inside binds the enclosing table) with its
// own bound arguments threaded through in text order. For the value-list
// form, value is always a []any leaf, produced by Column.In/NullableColumn.In.
func renderIn(d dialect.Dialect, n Node, counter *argCounter, scope renderScope) (clause string, args []any, err error) {
	col := quoteColumn(d, n.Column)

	kw := " IN ("
	if n.Op == OpNotIn {
		kw = " NOT IN ("
	}

	if sq, ok := n.Value.(Subquery); ok {
		if len(sq.Columns) != 1 {
			return "", nil, fmt.Errorf("orm/render: IN-subquery must project exactly one column, got %d (project one via the query's Columns selector)", len(sq.Columns))
		}

		innerSQL, innerArgs, err := renderSubquery(d, sq, counter, scope)
		if err != nil {
			return "", nil, err
		}

		return col + kw + innerSQL + ")", innerArgs, nil
	}

	vs, _ := n.Value.([]any)

	if len(vs) == 0 {
		// An empty IN() is never valid SQL; render a clause that is always
		// false rather than emitting syntactically broken text.
		return "1 = 0", nil, nil
	}

	phs := make([]string, len(vs))
	for i := range vs {
		phs[i] = d.Placeholder(counter.next())
	}

	return col + kw + strings.Join(phs, ", ") + ")", vs, nil
}

// renderSubquery renders a subquery's inner SELECT inline, continuing the
// ENCLOSING expression's placeholder counter (so Postgres-style numbered
// placeholders stay sequential across the whole statement) and returning
// its bound arguments in placeholder order. Called by renderIn (In/
// NotIn-subquery), renderBinary (scalar comparisons) and renderExists.
// The subquery's own scope is derived from scope: this subquery becomes the
// corr target for ITS nested subqueries, while THIS statement -- named by
// scope.stmt -- becomes this subquery's corr, so a marker in the subquery's
// WHERE binds the directly enclosing query.
func renderSubquery(d dialect.Dialect, sq Subquery, counter *argCounter, scope renderScope) (string, []any, error) {
	return renderSelectInto(d, sq.Table, sq.Columns, sq.Where, sq.Order, sq.Limit, sq.Offset, counter,
		renderScope{stmt: []string{sq.Table}, corr: scope.stmt}, SelectModifiers{})
}

// renderExists renders an `EXISTS (SELECT ...)` clause for a KindSubquery
// node built by the Exists/NotExists helpers. EXISTS accepts any projection (the
// column list is irrelevant to its truth value), so no single-column check
// applies. err is non-nil when n.Value is not a Subquery -- a malformed
// subquery node renders nothing usable.
func renderExists(d dialect.Dialect, n Node, counter *argCounter, scope renderScope) (clause string, args []any, err error) {
	sq, ok := n.Value.(Subquery)
	if !ok {
		return "", nil, fmt.Errorf("orm/render: subquery node value of type %T is not a Subquery", n.Value)
	}

	innerSQL, innerArgs, err := renderSubquery(d, sq, counter, scope)
	if err != nil {
		return "", nil, err
	}

	return "EXISTS (" + innerSQL + ")", innerArgs, nil
}

// renderTuple renders a KindTuple multi-column row-value predicate built by
// the Tuple API: a parenthesized LHS column list compared against an inner
// subquery. Op In/NotIn renders membership (`("a", "b") [NOT] IN (SELECT
// ...)`); Op Eq..Lte renders a row-value comparison (`("a", "b") OP (SELECT
// ...)`), valid on Postgres and SQLite (>= 3.15).
//
// The subquery renders inline through renderSubquery, continuing counter and
// the enclosing scope exactly as #399's In-subquery/scalar-comparison paths
// do, so a correlated WHERE inside the tuple subquery binds the enclosing
// single-table SELECT and placeholders stay sequential across the statement.
//
// Every failure mode is a typed rendering error, never silently-wrong SQL:
// an empty tuple (zero columns), a non-subquery Value, an arity mismatch
// between the tuple and the subquery's projection, and an operator that is
// neither membership nor a comparison.
func renderTuple(d dialect.Dialect, n Node, counter *argCounter, scope renderScope) (string, []any, error) {
	if len(n.Tuple) == 0 {
		return "", nil, errors.New("orm/render: tuple predicate requires at least one column")
	}

	sq, ok := n.Value.(Subquery)
	if !ok {
		return "", nil, fmt.Errorf("orm/render: tuple predicate requires a subquery operand, got %T", n.Value)
	}

	if len(sq.Columns) != len(n.Tuple) {
		return "", nil, fmt.Errorf("orm/render: tuple subquery arity mismatch: tuple has %d columns, subquery projects %d (match via the query's Columns selector)", len(n.Tuple), len(sq.Columns))
	}

	innerSQL, innerArgs, err := renderSubquery(d, sq, counter, scope)
	if err != nil {
		return "", nil, err
	}

	lhs := "(" + joinQuoted(d, n.Tuple) + ")"

	switch n.Op { //nolint:exhaustive // only membership and the comparison ops are valid on a tuple; the rest error in default
	case OpIn:
		return lhs + " IN (" + innerSQL + ")", innerArgs, nil
	case OpNotIn:
		return lhs + " NOT IN (" + innerSQL + ")", innerArgs, nil
	case OpEq, OpNeq, OpGt, OpGte, OpLt, OpLte:
		// The case list pre-validates the operator, so opSymbol cannot
		// fail here.
		symbol, _ := opSymbol(n.Op)

		return lhs + " " + symbol + " (" + innerSQL + ")", innerArgs, nil
	default:
		return "", nil, fmt.Errorf("orm/render: operator %d is not valid on a tuple predicate", n.Op)
	}
}

func renderCompound(d dialect.Dialect, n Node, counter *argCounter, scope renderScope) (clause string, args []any, err error) {
	if n.Compound == CompoundNot {
		var (
			childClause string
			childArgs   []any
		)

		if len(n.Children) == 1 {
			childClause, childArgs, err = renderExprCorr(d, n.Children[0], counter, scope)
			if err != nil {
				return "", nil, err
			}
		}

		return "NOT (" + childClause + ")", childArgs, nil
	}

	joiner := " AND "
	if n.Compound == CompoundOr {
		joiner = " OR "
	}

	parts := make([]string, 0, len(n.Children))

	for _, child := range n.Children {
		c, a, err := renderExprCorr(d, child, counter, scope)
		if err != nil {
			return "", nil, err
		}

		if c == "" {
			continue
		}

		parts = append(parts, c)
		args = append(args, a...)
	}

	return "(" + strings.Join(parts, joiner) + ")", args, nil
}

// renderFuncPredicate renders a KindFunc node: the scalar expression tree
// (renderFuncExpr) followed by the node's comparison against its bound
// value. It mirrors renderBinary's operator/placeholder plumbing, except the
// left-hand side may itself bind arguments (a COALESCE fallback, a NULLIF
// operand, a CASE's WHEN/THEN leaves), which are emitted -- and their
// arguments collected -- BEFORE the comparison placeholder, so text order
// and $N numbering stay consistent.
func renderFuncPredicate(d dialect.Dialect, n Node, counter *argCounter, scope renderScope) (string, []any, error) {
	lhs, largs, err := renderFuncExpr(d, n.Func, counter, scope)
	if err != nil {
		return "", nil, err
	}

	switch n.Op { //nolint:exhaustive // only the comparison ops are legal on an expression; the rest error in default
	case OpIsNull:
		return lhs + " IS NULL", largs, nil
	case OpIsNotNull:
		return lhs + " IS NOT NULL", largs, nil
	case OpIn:
		vs, _ := n.Value.([]any)

		if len(vs) == 0 {
			return "1 = 0", largs, nil
		}

		phs := make([]string, len(vs))
		for i := range vs {
			phs[i] = d.Placeholder(counter.next())
		}

		return lhs + " IN (" + strings.Join(phs, ", ") + ")", append(largs, vs...), nil
	case OpEq, OpNeq, OpGt, OpGte, OpLt, OpLte:
		// The case list pre-validates the operator, so opSymbol cannot
		// fail here.
		symbol, _ := opSymbol(n.Op)

		ph := d.Placeholder(counter.next())

		return lhs + " " + symbol + " " + ph, append(largs, n.Value), nil
	default:
		return "", nil, fmt.Errorf("orm/render: operator %d is not valid on a scalar expression", n.Op)
	}
}

// renderFuncExpr renders a scalar function or CASE expression tree and
// returns its bound arguments in placeholder/text order. A nil fn is a
// malformed node and fails closed rather than rendering an empty fragment.
func renderFuncExpr(d dialect.Dialect, fn *FuncExpr, counter *argCounter, scope renderScope) (string, []any, error) {
	if fn == nil {
		return "", nil, errors.New("orm/render: scalar expression node has no function payload")
	}

	if fn.Case {
		return renderCaseExpr(d, fn, counter, scope)
	}

	name := fn.Name
	if name == "" {
		return "", nil, errors.New("orm/render: scalar expression has an empty function name")
	}

	args := make([]string, len(fn.Args))

	var bound []any

	for i, a := range fn.Args {
		text, aargs, err := renderScalar(d, a, counter, scope)
		if err != nil {
			return "", nil, err
		}

		args[i] = text

		bound = append(bound, aargs...)
	}

	return name + "(" + strings.Join(args, ", ") + ")", bound, nil
}

// renderScalar renders one scalar-expression argument: a quoted column, a
// placeholder-bound literal, or a nested function expression. It is the
// argument-level counterpart to renderFuncPredicate (which renders the
// comparison around a top-level expression). err is non-nil for a Node kind
// that is not part of the scalar-expression algebra -- never produced by
// the expression builder, but a malformed caller-built node must not produce empty SQL.
func renderScalar(d dialect.Dialect, n Node, counter *argCounter, scope renderScope) (string, []any, error) {
	switch n.Kind { //nolint:exhaustive // only the scalar-expression kinds are legal; the rest error in default
	case KindColumn:
		return quoteColumn(d, n.Column), nil, nil
	case KindLit:
		return d.Placeholder(counter.next()), []any{n.Value}, nil
	case KindFunc:
		return renderFuncExpr(d, n.Func, counter, scope)
	case KindSubquery:
		sq, ok := n.Value.(Subquery)
		if !ok {
			return "", nil, fmt.Errorf("orm/render: scalar subquery kind carries value of type %T, not a Subquery", n.Value)
		}

		if len(sq.Columns) != 1 {
			return "", nil, fmt.Errorf("orm/render: scalar subquery must project exactly one column, got %d (project one via the query's Columns selector)", len(sq.Columns))
		}

		innerSQL, innerArgs, err := renderSubquery(d, sq, counter, scope)
		if err != nil {
			return "", nil, err
		}

		return "(" + innerSQL + ")", innerArgs, nil
	default:
		return "", nil, fmt.Errorf("orm/render: unsupported scalar expression argument kind %d", n.Kind)
	}
}

// renderCaseExpr renders a searched CASE expression:
// `CASE WHEN <cond> THEN <scalar> [ELSE <scalar>] END`. Each WHEN condition
// renders through the ordinary predicate renderer (so it may bind
// arguments) and each THEN/ELSE is a scalar expression; all are emitted in
// text order. A CASE with no WHEN arm (unrepresentable through the Case builder's
// OngoingCase, which guarantees at least one) or an empty WHEN condition is
// a rendering error, never syntactically broken SQL.
func renderCaseExpr(d dialect.Dialect, fn *FuncExpr, counter *argCounter, scope renderScope) (string, []any, error) {
	if len(fn.Whens) == 0 {
		return "", nil, errors.New("orm/render: CASE expression requires at least one WHEN arm")
	}

	var (
		b     strings.Builder
		bound []any
	)

	b.WriteString("CASE")

	for _, w := range fn.Whens {
		cond, cargs, err := renderExprCorr(d, w.Cond, counter, scope)
		if err != nil {
			return "", nil, err
		}

		if cond == "" {
			return "", nil, errors.New("orm/render: CASE WHEN arm has an empty condition")
		}

		thenText, targs, err := renderScalar(d, w.Then, counter, scope)
		if err != nil {
			return "", nil, err
		}

		b.WriteString(" WHEN ")
		b.WriteString(cond)
		b.WriteString(" THEN ")
		b.WriteString(thenText)

		bound = append(bound, cargs...)
		bound = append(bound, targs...)
	}

	if fn.Else != nil {
		elseText, eargs, err := renderScalar(d, *fn.Else, counter, scope)
		if err != nil {
			return "", nil, err
		}

		b.WriteString(" ELSE ")
		b.WriteString(elseText)

		bound = append(bound, eargs...)
	}

	b.WriteString(" END")

	return b.String(), bound, nil
}

func opSymbol(op Op) (string, error) {
	switch op { //nolint:exhaustive // array quantifier ops are handled by renderArray; the default rejects the rest
	case OpEq:
		return "=", nil
	case OpNeq:
		return "!=", nil
	case OpGt:
		return ">", nil
	case OpGte:
		return ">=", nil
	case OpLt:
		return "<", nil
	case OpLte:
		return "<=", nil
	case OpLike:
		return "LIKE", nil
	case OpBetween:
		return "BETWEEN", nil
	case OpIn, OpNotIn, OpIsNull, OpIsNotNull:
		// Handled separately in their own render funcs; never reaches
		// opSymbol.
		return "=", nil
	default:
		return "", fmt.Errorf("orm/render: unknown operator %d", op)
	}
}

// ErrUnsupported is returned by capability-gated renders when the resolved
// dialect lacks a feature the caller asked for -- e.g. an aggregate whose
// SQL function has no equivalent on that dialect (see agg.go). Callers test
// with errors.Is.
var ErrUnsupported = errors.New("orm/render: unsupported by dialect")
