// Package render (this file) extends the package with GROUP BY/aggregate
// SELECT rendering, ported from orm/engine/aggregate.go's RenderAggregate
// (read in full while designing this), plus HAVING rendering -- genuinely
// new, since the old ORM has zero HAVING support anywhere (grep-confirmed
// before writing this file). See the aggregate builder for the public, typed builder
// surface that calls into GroupedSelect.
package render

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/zenta-dev/zever/orm/dialect"
)

// AggFunc mirrors the builder's AggFunc values (same underlying int representation
// and ordering as orm/engine.AggFunc before it), so a caller-side
// `render.AggFunc(a.Func)` plain conversion is always legal.
type AggFunc int

// Supported aggregate functions; order and values match the builder's AggFunc.
const (
	AggCount AggFunc = iota
	AggSum
	AggAvg
	AggMin
	AggMax
	AggArrayAgg
	AggStringAgg
	AggGroupConcat
)

func (f AggFunc) name() string {
	switch f { //nolint:exhaustive // the default guards unknown func values
	case AggCount:
		return "COUNT"
	case AggSum:
		return "SUM"
	case AggAvg:
		return "AVG"
	case AggMin:
		return "MIN"
	case AggMax:
		return "MAX"
	case AggArrayAgg:
		return "array_agg"
	case AggStringAgg:
		return "string_agg"
	case AggGroupConcat:
		return "group_concat"
	default:
		return "COUNT"
	}
}

// numeric reports whether f always produces a numeric result, i.e. HAVING
// comparisons against it should reject an obviously-non-numeric Value. See
// this file's HAVING type-check note below for why this is the narrowest
// check available.
func (f AggFunc) numeric() bool {
	switch f { //nolint:exhaustive // the default guards unknown func values
	case AggCount, AggSum, AggAvg:
		return true
	case AggMin, AggMax:
		// MIN/MAX are meaningful over any orderable type (string, timestamp,
		// numeric); HAVING comparisons against them are intentionally not
		// restricted to numeric values.
		return false
	default:
		return false
	}
}

// OrderRef is the erased shape of one argument-last ORDER BY term inside an
// ordered aggregate: a plain identifier plus its direction.
type OrderRef struct {
	Column string
	Desc   bool
}

// Aggregate describes one aggregate expression, e.g.
// {Func: AggSum, Column: "amount_cents", Alias: "sum_amount_cents"} renders
// as `SUM("amount_cents") AS "sum_amount_cents"` in a select list, or
// `SUM("amount_cents")` (no alias, no AS) when used as a HAVING operand. An
// empty Column means "*" (only meaningful for AggCount).
//
// DistinctArg renders the argument as `DISTINCT <col>` (COUNT(DISTINCT
// col)); Filter, when non-nil, renders a trailing
// `FILTER (WHERE <predicate>)` clause. Both are deliberately plain data --
// render never imports the orm root, so the builder's Aggregate distinct/filter
// state is erased into these fields by the caller.
//
// Delimiter/HasDelimiter carry the string_agg/group_concat delimiter (the
// bool distinguishes "unset" from an empty delimiter); Order carries the
// argument-last ORDER BY of an ordered aggregate; Invalid, when non-empty, is
// a caller error surfaced as a typed rendering error rather than silently
// dropped (see orderedAgg).
type Aggregate struct {
	Func         AggFunc
	Column       string
	Alias        string
	DistinctArg  bool
	Filter       *Node
	Delimiter    string
	HasDelimiter bool
	Order        []OrderRef
	Invalid      string
}

// selectExpr renders a as it appears in the SELECT list (with its alias),
// returning any arguments bound by its FILTER predicate.
func (a Aggregate) selectExpr(d dialect.Dialect, counter *argCounter) (string, []any, error) {
	out, args, err := a.callExpr(d, counter)
	if err != nil {
		return "", nil, err
	}

	if a.Alias != "" {
		out += " AS " + d.QuoteIdent(a.Alias)
	}

	return out, args, nil
}

// callExpr renders a's bare function-call expression, with no alias and no
// AS -- what's needed inside a HAVING clause and inside a SELECT list. It
// renders the DISTINCT argument modifier and the optional FILTER clause,
// collecting the FILTER predicate's bound arguments in text order. err is
// non-nil for a DISTINCT with no column, or a FILTER on a dialect that has
// no FILTER clause (typed dialect.ErrUnsupportedByDialect).
func (a Aggregate) callExpr(d dialect.Dialect, counter *argCounter) (string, []any, error) {
	out, args, err := a.baseCall(d, counter)
	if err != nil {
		return "", nil, err
	}

	if a.Filter == nil {
		return out, args, nil
	}

	if !supportsAggregateFilter(d) {
		return "", nil, unsupportedByDialect(d, "aggregate FILTER (WHERE ...)")
	}

	clause, filterArgs, err := renderExpr(d, *a.Filter, counter)
	if err != nil {
		return "", nil, err
	}

	if clause == "" {
		return "", nil, fmt.Errorf("%w: aggregate FILTER (WHERE ...) has an empty predicate", ErrUnsupported)
	}

	return out + " FILTER (WHERE " + clause + ")", append(args, filterArgs...), nil
}

// baseCall renders a's function-call expression without its FILTER clause:
// either one of the ordered/concatenating aggregates, or the COUNT/SUM/AVG/
// MIN/MAX family with the DISTINCT argument modifier.
func (a Aggregate) baseCall(d dialect.Dialect, counter *argCounter) (string, []any, error) {
	switch a.Func { //nolint:exhaustive // the ordered kinds are handled here; the rest fall through to the default path
	case AggArrayAgg, AggStringAgg, AggGroupConcat:
		return a.orderedAgg(d, counter)
	}

	arg := "*"

	switch {
	case a.DistinctArg && a.Column == "":
		return "", nil, fmt.Errorf("%w: %s(DISTINCT) requires a column", ErrUnsupported, a.Func.name())
	case a.DistinctArg:
		arg = "DISTINCT " + quoteColumn(d, a.Column)
	case a.Column != "":
		arg = quoteColumn(d, a.Column)
	}

	return a.Func.name() + "(" + arg + ")", nil, nil
}

// orderedAgg renders an ordered/concatenating aggregate:
//
//	Postgres: array_agg(x [ORDER BY ...]) / string_agg(x, ? [ORDER BY ...])
//	SQLite:   group_concat(x [, ?] [ORDER BY ...])
//
// Each function family is gated by dialect.OrderedAggregateDialect (typed
// dialect.ErrUnsupportedByDialect on a mismatch) and the in-aggregate ORDER
// BY is gated separately -- SQLite only from 3.44.0. The delimiter is a
// bound placeholder. err is non-nil for an invalid erased aggregate, a
// missing column, or any unsupported combination.
func (a Aggregate) orderedAgg(d dialect.Dialect, counter *argCounter) (string, []any, error) {
	if a.Invalid != "" {
		return "", nil, fmt.Errorf("%w: %s", ErrUnsupported, a.Invalid)
	}

	if a.Column == "" {
		return "", nil, fmt.Errorf("%w: %s requires a column", ErrUnsupported, a.Func.name())
	}

	fn, err := orderedAggFuncName(d, a.Func)
	if err != nil {
		return "", nil, err
	}

	arg := quoteColumn(d, a.Column)
	if a.DistinctArg {
		arg = "DISTINCT " + arg
	}

	var (
		out  strings.Builder
		args []any
	)

	out.WriteString(fn)
	out.WriteString("(")
	out.WriteString(arg)

	// Postgres string_agg and SQLite group_concat take the delimiter as a
	// normal second argument (bound).
	if a.Func != AggArrayAgg && a.HasDelimiter {
		out.WriteString(", ")
		out.WriteString(d.Placeholder(counter.next()))

		args = append(args, a.Delimiter)
	}

	if err := writeAggOrderBy(&out, d, a.Order); err != nil {
		return "", nil, err
	}

	out.WriteString(")")

	return out.String(), args, nil
}

// orderedAggFuncName validates d's support for a's ordered aggregate family
// and returns the rendered function name. The three function families are
// disjoint (array_agg/string_agg are Postgres-only, group_concat is
// SQLite-only), so a mismatch is a typed
// dialect.ErrUnsupportedByDialect. err is non-nil for an unsupported family
// or a func that is not an ordered aggregate at all.
func orderedAggFuncName(d dialect.Dialect, f AggFunc) (string, error) {
	switch f { //nolint:exhaustive // only the three ordered kinds reach here; the default rejects the rest
	case AggArrayAgg:
		if !supportsArrayAgg(d) {
			return "", unsupportedByDialect(d, "array_agg")
		}

		return "array_agg", nil
	case AggStringAgg:
		if !supportsStringAgg(d) {
			return "", unsupportedByDialect(d, "string_agg")
		}

		return "string_agg", nil
	case AggGroupConcat:
		if !supportsGroupConcat(d) {
			return "", unsupportedByDialect(d, "GROUP_CONCAT/group_concat")
		}

		return "group_concat", nil
	default:
		return "", fmt.Errorf("%w: %s is not an ordered aggregate", ErrUnsupported, f.name())
	}
}

// writeAggOrderBy writes an ordered aggregate's argument-last ORDER BY into
// out, gating the clause on dialect.OrderedAggregateDialect (SQLite from
// 3.44.0). An empty column is a typed rendering error; no order terms writes
// nothing.
func writeAggOrderBy(out *strings.Builder, d dialect.Dialect, order []OrderRef) error {
	if len(order) == 0 {
		return nil
	}

	if !supportsOrderedAggregates(d) {
		return unsupportedByDialect(d, "ORDER BY inside an aggregate")
	}

	out.WriteString(" ORDER BY ")

	for i, o := range order {
		if i > 0 {
			out.WriteString(", ")
		}

		if o.Column == "" {
			return fmt.Errorf("%w: ordered aggregate ORDER BY has an empty column", ErrUnsupported)
		}

		out.WriteString(quoteColumn(d, o.Column))

		if o.Desc {
			out.WriteString(" DESC")
		} else {
			out.WriteString(" ASC")
		}
	}

	return nil
}

// exprOnly renders a's bare function-call expression without the
// DISTINCT/FILTER extensions. It is retained for the window renderer
// (render/window.go), which has no FILTER/DISTINCT surface; grouped SELECT
// and HAVING render through callExpr instead.
func (a Aggregate) exprOnly(d dialect.Dialect) string {
	arg := "*"
	if a.Column != "" {
		arg = quoteColumn(d, a.Column)
	}

	return a.Func.name() + "(" + arg + ")"
}

// GroupKind discriminates the shape of a GroupTerm: a plain group
// expression, or one of the ROLLUP/CUBE/GROUPING SETS constructs.
type GroupKind int

// Supported group-term kinds.
const (
	GroupPlain GroupKind = iota
	GroupRollup
	GroupCube
	GroupGroupingSets
)

// GroupTerm is the erased shape of one GROUP BY element: a plain column or
// scalar expression (Column or Func set), a ROLLUP/CUBE over Terms, or a
// GROUPING SETS list of Sets. It mirrors the builder's unexported grouping-term
// model, converted to render's own shape just before rendering.
type GroupTerm struct {
	Kind   GroupKind
	Column string
	Func   *FuncExpr
	Terms  []GroupTerm
	Sets   [][]GroupTerm
}

// HavingKind discriminates the shape of a HavingNode, mirroring Node/
// NodeKind's leaf/compound split but over aggregate expressions instead of
// real columns. HavingExpr is a general scalar predicate (a scalar-expression predicate rooted
// into a Predicate), rendered through the ordinary predicate renderer.
type HavingKind int

// Supported HAVING node kinds.
const (
	HavingNone HavingKind = iota
	HavingLeaf
	HavingCompound
	HavingExpr
)

// HavingNode is the erased shape of one HAVING predicate tree, structurally
// parallel to Node but deliberately a SEPARATE type rather than a reuse of
// Node's NFunc kind: a HAVING leaf compares an *aggregate function
// expression* (Agg) to Value, not a real Column -- Node has no field to
// carry the aggregate function itself. HavingExpr carries a general scalar
// predicate Node in Expr (a grouped-column function/expression comparison).
type HavingNode struct {
	Kind     HavingKind
	Agg      Aggregate
	Op       Op
	Value    any
	Compound CompoundOp
	Children []HavingNode
	Expr     *Node
}

// GroupedSelect renders a grouped aggregate SELECT:
//
//	SELECT <group leaves...>, <agg exprs...> FROM <table> [WHERE ...] GROUP BY <groups...> [HAVING ...]
//
// groups are the GROUP BY specification; their plain leaves (columns and
// scalar expressions, flattened through ROLLUP/CUBE/GROUPING SETS) are
// emitted first in the select list, then every aggregate, exactly mirroring
// RenderAggregate's (the old ORM's) select-list order -- callers scan result
// rows in that same order. where and having share ONE ordered
// positional-argument list in SQL text order: select-list args (group
// expressions + aggregate FILTERs), then WHERE's, then GROUP BY's, then
// HAVING's.
//
// err is non-nil for a HAVING leaf whose Value fails the type check
// documented on renderHavingLeaf, a DISTINCT aggregate with no column, or a
// grouping construct the dialect lacks (typed
// dialect.ErrUnsupportedByDialect) -- a rendering-time error, never a panic.
func GroupedSelect(
	d dialect.Dialect,
	table string,
	groups []GroupTerm,
	aggs []Aggregate,
	where Node,
	having HavingNode,
) (query string, args []any, err error) {
	counter := &argCounter{}

	parts := make([]string, 0, len(groups)+len(aggs))

	for _, leaf := range flattenGroupTerms(groups) {
		text, leafArgs, leafErr := renderGroupLeaf(d, leaf, counter)
		if leafErr != nil {
			return "", nil, leafErr
		}

		parts = append(parts, text)
		args = append(args, leafArgs...)
	}

	for _, a := range aggs {
		text, aggArgs, aggErr := a.selectExpr(d, counter)
		if aggErr != nil {
			return "", nil, aggErr
		}

		parts = append(parts, text)
		args = append(args, aggArgs...)
	}

	if len(parts) == 0 {
		parts = append(parts, "COUNT(*)")
	}

	var b strings.Builder

	b.WriteString("SELECT ")
	b.WriteString(strings.Join(parts, ", "))
	b.WriteString(" FROM ")
	b.WriteString(d.QuoteIdent(table))

	whereClause, whereArgs, err := renderExpr(d, where, counter)
	if err != nil {
		return "", nil, err
	}

	if whereClause != "" {
		b.WriteString(" WHERE ")
		b.WriteString(whereClause)
	}

	args = append(args, whereArgs...)

	groupClause, groupArgs, err := renderGroupBy(d, groups, counter)
	if err != nil {
		return "", nil, err
	}

	if groupClause != "" {
		b.WriteString(" GROUP BY ")
		b.WriteString(groupClause)
	}

	args = append(args, groupArgs...)

	havingClause, havingArgs, err := renderHaving(d, having, counter)
	if err != nil {
		return "", nil, err
	}

	if havingClause != "" {
		b.WriteString(" HAVING ")
		b.WriteString(havingClause)
	}

	args = append(args, havingArgs...)

	return b.String(), args, nil
}

// flattenGroupTerms returns the select-list leaves a GROUP BY specification
// projects: every plain column/expression, in first-appearance order, with
// duplicates removed (a grouping construct may name the same column in more
// than one set). It walks ROLLUP/CUBE Terms and GROUPING SETS Sets
// recursively.
func flattenGroupTerms(groups []GroupTerm) []GroupTerm {
	var (
		out  []GroupTerm
		seen = map[string]struct{}{}
	)

	var walk func(g GroupTerm)

	walk = func(g GroupTerm) {
		switch g.Kind { //nolint:exhaustive // GroupPlain is the default leaf; the rest recurse
		case GroupRollup, GroupCube:
			for _, t := range g.Terms {
				walk(t)
			}
		case GroupGroupingSets:
			for _, set := range g.Sets {
				for _, t := range set {
					walk(t)
				}
			}
		default:
			key := groupLeafKey(g)
			if _, ok := seen[key]; ok {
				return
			}

			seen[key] = struct{}{}

			out = append(out, g)
		}
	}

	for _, g := range groups {
		walk(g)
	}

	return out
}

// groupLeafKey returns a dedup key for a plain group leaf: the column name
// for a plain column, or the function tree's pointer identity for an
// expression.
func groupLeafKey(g GroupTerm) string {
	if g.Func != nil {
		return fmt.Sprintf("f:%p", g.Func)
	}

	return "c:" + g.Column
}

// renderGroupLeaf renders one plain group leaf: a quoted column or a scalar
// expression with its bound arguments.
func renderGroupLeaf(d dialect.Dialect, g GroupTerm, counter *argCounter) (string, []any, error) {
	if g.Func != nil {
		return renderFuncExpr(d, g.Func, counter, renderScope{})
	}

	if g.Column == "" {
		return "", nil, fmt.Errorf("%w: group term has neither a column nor an expression", ErrUnsupported)
	}

	return quoteColumn(d, g.Column), nil, nil
}

// renderGroupBy renders a GROUP BY clause body, returning its bound
// arguments. Each ROLLUP/CUBE/GROUPING SETS construct is gated on its
// per-dialect capability.
func renderGroupBy(d dialect.Dialect, groups []GroupTerm, counter *argCounter) (clause string, args []any, err error) {
	if len(groups) == 0 {
		return "", nil, nil
	}

	parts := make([]string, 0, len(groups))

	for _, g := range groups {
		text, gargs, gErr := renderGroupTerm(d, g, counter)
		if gErr != nil {
			return "", nil, gErr
		}

		parts = append(parts, text)
		args = append(args, gargs...)
	}

	return strings.Join(parts, ", "), args, nil
}

// renderGroupTerm renders one GROUP BY element, gating each ROLLUP/CUBE/
// GROUPING SETS construct on its per-dialect capability.
func renderGroupTerm(d dialect.Dialect, g GroupTerm, counter *argCounter) (string, []any, error) {
	switch g.Kind {
	case GroupPlain:
		return renderGroupLeaf(d, g, counter)
	case GroupRollup:
		if !supportsRollup(d) {
			return "", nil, unsupportedByDialect(d, "ROLLUP")
		}

		return renderKeywordGroup(d, "ROLLUP", g.Terms, counter)
	case GroupCube:
		if !supportsCube(d) {
			return "", nil, unsupportedByDialect(d, "CUBE")
		}

		return renderKeywordGroup(d, "CUBE", g.Terms, counter)
	case GroupGroupingSets:
		if !supportsGroupingSets(d) {
			return "", nil, unsupportedByDialect(d, "GROUPING SETS")
		}

		return renderGroupingSets(d, g.Sets, counter)
	default:
		return "", nil, fmt.Errorf("orm/render: unknown group term kind %d", g.Kind)
	}
}

// renderKeywordGroup renders `ROLLUP(...)` / `CUBE(...)`: a comma-separated
// list of plain group leaves. Nested grouping constructs are rejected rather
// than rendered into invalid SQL.
func renderKeywordGroup(d dialect.Dialect, keyword string, terms []GroupTerm, counter *argCounter) (string, []any, error) {
	if len(terms) == 0 {
		return "", nil, fmt.Errorf("%w: %s requires at least one grouping term", ErrUnsupported, keyword)
	}

	parts := make([]string, 0, len(terms))

	var args []any

	for _, t := range terms {
		if t.Kind != GroupPlain {
			return "", nil, fmt.Errorf("%w: nested grouping constructs inside %s are unsupported", ErrUnsupported, keyword)
		}

		text, targs, err := renderGroupLeaf(d, t, counter)
		if err != nil {
			return "", nil, err
		}

		parts = append(parts, text)
		args = append(args, targs...)
	}

	return keyword + "(" + strings.Join(parts, ", ") + ")", args, nil
}

// renderGroupingSets renders `GROUPING SETS ((...), (...))`, preserving each
// set's order (an empty set renders `()` -- the grand total).
func renderGroupingSets(d dialect.Dialect, sets [][]GroupTerm, counter *argCounter) (string, []any, error) {
	if len(sets) == 0 {
		return "", nil, fmt.Errorf("%w: GROUPING SETS requires at least one set", ErrUnsupported)
	}

	parts := make([]string, 0, len(sets))

	var args []any

	for _, set := range sets {
		inner := make([]string, 0, len(set))

		for _, t := range set {
			if t.Kind != GroupPlain {
				return "", nil, fmt.Errorf("%w: nested grouping constructs inside GROUPING SETS are unsupported", ErrUnsupported)
			}

			text, targs, err := renderGroupLeaf(d, t, counter)
			if err != nil {
				return "", nil, err
			}

			inner = append(inner, text)
			args = append(args, targs...)
		}

		parts = append(parts, "("+strings.Join(inner, ", ")+")")
	}

	return "GROUPING SETS (" + strings.Join(parts, ", ") + ")", args, nil
}

// renderHaving walks a HavingNode tree, continuing counter from wherever
// WHERE's rendering left it off so placeholder numbers stay sequential
// across the whole statement (matters for Postgres's $N-numbered
// placeholders).
func renderHaving(d dialect.Dialect, n HavingNode, counter *argCounter) (clause string, args []any, err error) {
	switch n.Kind {
	case HavingNone:
		return "", nil, nil
	case HavingLeaf:
		return renderHavingLeaf(d, n, counter)
	case HavingCompound:
		return renderHavingCompound(d, n, counter)
	case HavingExpr:
		if n.Expr == nil {
			return "", nil, fmt.Errorf("%w: HAVING expression has no predicate", ErrUnsupported)
		}

		return renderExpr(d, *n.Expr, counter)
	default:
		return "", nil, nil
	}
}

// renderHavingLeaf renders one `<agg expr> <op> <placeholder>` clause. The
// aggregate expression itself may bind arguments (a FILTER (WHERE ...)
// predicate); those are emitted before the comparison placeholder, so text
// order and $N numbering stay consistent.
//
// Type-safety tradeoff (documented per DESIGN.md's instruction to flag
// tension between full typing and SQL coverage, and propose the narrowest
// escape hatch): Aggregate is a plain, non-generic struct -- by the time an
// Aggregate reaches here, its constructing Column's value type V has
// already been erased (this is deliberate: a single []Aggregate must be
// able to hold e.g. both a Sum[int64] and a Min[string] in one
// GroupBy(...).Agg(...) call, so Aggregate cannot itself carry a type
// parameter). That means a HAVING comparison can only be checked as
// strictly as the *erased* Aggregate allows: for AggFuncs that are always
// numeric-producing (COUNT/SUM/AVG), Value is checked to be a numeric Go
// kind and a rendering-time error (never a panic) is returned on mismatch;
// for MIN/MAX -- meaningful over any orderable type, string/timestamp
// included -- no such check is possible or attempted. This is narrower than
// rejecting all untyped comparisons outright (which would make MIN/MAX
// HAVING clauses unspellable) and stops well short of a broader "accept
// literally anything" weakening.
func renderHavingLeaf(d dialect.Dialect, n HavingNode, counter *argCounter) (clause string, args []any, err error) {
	symbol, err := opSymbol(n.Op)
	if err != nil {
		return "", nil, err
	}

	if n.Agg.Func.numeric() && !isNumericValue(n.Value) {
		return "", nil, fmt.Errorf(
			"%w: HAVING %s(%s) %s: value %v (%T) is not numeric",
			ErrUnsupported, n.Agg.Func.name(), n.Agg.Column, symbol, n.Value, n.Value,
		)
	}

	expr, exprArgs, err := n.Agg.callExpr(d, counter)
	if err != nil {
		return "", nil, err
	}

	ph := d.Placeholder(counter.next())

	return expr + " " + symbol + " " + ph, append(exprArgs, n.Value), nil
}

func renderHavingCompound(d dialect.Dialect, n HavingNode, counter *argCounter) (clause string, args []any, err error) {
	if n.Compound == CompoundNot {
		if len(n.Children) != 1 {
			return "", nil, nil
		}

		childClause, childArgs, childErr := renderHaving(d, n.Children[0], counter)
		if childErr != nil {
			return "", nil, childErr
		}

		return "NOT (" + childClause + ")", childArgs, nil
	}

	joiner := " AND "
	if n.Compound == CompoundOr {
		joiner = " OR "
	}

	parts := make([]string, 0, len(n.Children))

	for _, child := range n.Children {
		c, a, childErr := renderHaving(d, child, counter)
		if childErr != nil {
			return "", nil, childErr
		}

		if c == "" {
			continue
		}

		parts = append(parts, c)
		args = append(args, a...)
	}

	return "(" + strings.Join(parts, joiner) + ")", args, nil
}

// isNumericValue reports whether v is a Go kind that can bind to a numeric
// SQL comparison -- any int/uint/float kind, or a numeric string literal
// (accepted since driver value conversion is lenient about numeric strings
// too). It deliberately does NOT attempt to recover the aggregate's exact
// declared V (impossible post-erasure, see renderHavingLeaf's doc comment)
// -- only to reject obviously-wrong values like a bool or a struct.
func isNumericValue(v any) bool {
	switch x := v.(type) {
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return true
	case string:
		_, err := strconv.ParseFloat(x, 64)

		return err == nil
	default:
		return false
	}
}

// aggregateGrouping returns d's AggregateGroupingDialect capability set, if
// it implements one. A dialect that does not implement the interface at all
// (e.g. a base-only test dialect) is treated as supporting none of them,
// matching the repo's capability-gate convention.
func aggregateGrouping(d dialect.Dialect) (dialect.AggregateGroupingDialect, bool) {
	g, ok := d.(dialect.AggregateGroupingDialect)

	return g, ok
}

func supportsAggregateFilter(d dialect.Dialect) bool {
	g, ok := aggregateGrouping(d)

	return ok && g.SupportsAggregateFilter()
}

func supportsRollup(d dialect.Dialect) bool {
	g, ok := aggregateGrouping(d)

	return ok && g.SupportsRollup()
}

func supportsCube(d dialect.Dialect) bool {
	g, ok := aggregateGrouping(d)

	return ok && g.SupportsCube()
}

func supportsGroupingSets(d dialect.Dialect) bool {
	g, ok := aggregateGrouping(d)

	return ok && g.SupportsGroupingSets()
}

// orderedAggregate returns d's OrderedAggregateDialect capability set, if it
// implements one. A dialect that does not implement the interface at all
// (e.g. a base-only test dialect) is treated as supporting none of them,
// matching the repo's capability-gate convention.
func orderedAggregate(d dialect.Dialect) (dialect.OrderedAggregateDialect, bool) {
	od, ok := d.(dialect.OrderedAggregateDialect)

	return od, ok
}

func supportsArrayAgg(d dialect.Dialect) bool {
	od, ok := orderedAggregate(d)

	return ok && od.SupportsArrayAgg()
}

func supportsStringAgg(d dialect.Dialect) bool {
	od, ok := orderedAggregate(d)

	return ok && od.SupportsStringAgg()
}

func supportsGroupConcat(d dialect.Dialect) bool {
	od, ok := orderedAggregate(d)

	return ok && od.SupportsGroupConcat()
}

func supportsOrderedAggregates(d dialect.Dialect) bool {
	od, ok := orderedAggregate(d)

	return ok && od.SupportsOrderedAggregates()
}

// unsupportedByDialect builds the typed capability-gate error for a group
// feature the resolved dialect lacks.
func unsupportedByDialect(d dialect.Dialect, feature string) error {
	return fmt.Errorf("orm/render: %w: dialect %q does not support %s", dialect.ErrUnsupportedByDialect, d.Name(), feature)
}
