// Package render (this file) extends the package with window-function
// expression rendering for the window builder: scalar window functions
// (ROW_NUMBER/RANK/DENSE_RANK/LEAD/LAG/NTILE) and aggregate-over-window
// expressions (SUM(...) OVER (...) etc.), each with an optional
// OVER (PARTITION BY ... ORDER BY ...) clause, rendered into a SELECT list
// alongside the entity's own columns.
package render

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/zenta-dev/zever/orm/dialect"
)

// WindowFunc mirrors the builder's WindowFunc values (same underlying int
// representation), so a caller-side `render.WindowFunc(e.fn)` plain
// conversion is always legal.
type WindowFunc int

// Supported scalar window functions. WinNone is the zero value: a
// WindowExpr with Func == WinNone carries an aggregate-over-window (its Agg
// field) instead.
const (
	WinNone WindowFunc = iota
	WinRowNumber
	WinRank
	WinDenseRank
	WinLead
	WinLag
	WinNTile
)

func (f WindowFunc) name() string {
	switch f {
	case WinNone:
		return ""
	case WinRowNumber:
		return "ROW_NUMBER"
	case WinRank:
		return "RANK"
	case WinDenseRank:
		return "DENSE_RANK"
	case WinLead:
		return "LEAD"
	case WinLag:
		return "LAG"
	case WinNTile:
		return "NTILE"
	default:
		return ""
	}
}

// FrameMode names a window frame's unit: ROWS, RANGE or GROUPS. FrameNone
// is the zero value -- a window with no explicit frame renders no frame
// clause at all, preserving the pre-frames behavior.
type FrameMode int

// Supported window frame modes.
const (
	FrameNone FrameMode = iota
	FrameRows
	FrameRange
	FrameGroups
)

// keyword returns the SQL keyword for the mode, or "" for FrameNone.
func (m FrameMode) keyword() string {
	switch m {
	case FrameNone:
		return ""
	case FrameRows:
		return "ROWS"
	case FrameRange:
		return "RANGE"
	case FrameGroups:
		return "GROUPS"
	default:
		return ""
	}
}

// FrameBoundKind identifies which of the five SQL frame bounds a FrameBound
// is, mirroring the builder's FrameBoundKind values.
type FrameBoundKind int

// The five SQL frame bounds, in their canonical SQL ordering (the order
// used to validate that a start bound does not sort after its end).
const (
	BoundUnboundedPreceding FrameBoundKind = iota
	BoundPreceding
	BoundCurrentRow
	BoundFollowing
	BoundUnboundedFollowing
)

// FrameBound is one endpoint of a window frame. N is the offset for
// BoundPreceding/BoundFollowing and is ignored by the other kinds.
type FrameBound struct {
	Kind FrameBoundKind
	N    int
}

// UnboundedPreceding returns the `UNBOUNDED PRECEDING` frame bound.
func UnboundedPreceding() FrameBound {
	return FrameBound{Kind: BoundUnboundedPreceding}
}

// Preceding returns an `n PRECEDING` frame bound. n must be >= 0; a
// negative offset is a rendering-time error.
func Preceding(n int) FrameBound {
	return FrameBound{Kind: BoundPreceding, N: n}
}

// CurrentRow returns the `CURRENT ROW` frame bound.
func CurrentRow() FrameBound {
	return FrameBound{Kind: BoundCurrentRow}
}

// Following returns an `n FOLLOWING` frame bound. n must be >= 0; a
// negative offset is a rendering-time error.
func Following(n int) FrameBound {
	return FrameBound{Kind: BoundFollowing, N: n}
}

// UnboundedFollowing returns the `UNBOUNDED FOLLOWING` frame bound.
func UnboundedFollowing() FrameBound {
	return FrameBound{Kind: BoundUnboundedFollowing}
}

// frameBoundText renders b's SQL text, validating its offset and kind. A
// negative Preceding/Following offset or an out-of-range kind is a typed
// rendering error, never invalid SQL.
func frameBoundText(b FrameBound) (string, error) {
	switch b.Kind {
	case BoundUnboundedPreceding:
		return "UNBOUNDED PRECEDING", nil
	case BoundPreceding:
		if b.N < 0 {
			return "", fmt.Errorf("orm/render: PRECEDING offset %d must be >= 0", b.N)
		}

		return strconv.Itoa(b.N) + " PRECEDING", nil
	case BoundCurrentRow:
		return "CURRENT ROW", nil
	case BoundFollowing:
		if b.N < 0 {
			return "", fmt.Errorf("orm/render: FOLLOWING offset %d must be >= 0", b.N)
		}

		return strconv.Itoa(b.N) + " FOLLOWING", nil
	case BoundUnboundedFollowing:
		return "UNBOUNDED FOLLOWING", nil
	default:
		return "", fmt.Errorf("orm/render: unknown window frame bound kind %d", b.Kind)
	}
}

// frameBoundRank returns a bound kind's position in the canonical SQL frame
// ordering, or -1 for an out-of-range kind. Used to reject a frame whose
// start sorts after its end (e.g. `FOLLOWING 1 AND PRECEDING 1`).
func frameBoundRank(k FrameBoundKind) int {
	if k < BoundUnboundedPreceding || k > BoundUnboundedFollowing {
		return -1
	}

	return int(k)
}

// supportsWindowFrameRows reports whether d implements
// dialect.WindowFrameDialect and reports ROWS support. A dialect that does
// not implement the interface at all (e.g. a base-only dialect) is treated
// as unsupported, matching the repo's capability-gate convention.
func supportsWindowFrameRows(d dialect.Dialect) bool {
	wd, ok := d.(dialect.WindowFrameDialect)

	return ok && wd.SupportsWindowFrameRows()
}

// supportsWindowFrameRange reports whether d supports a RANGE frame; see
// supportsWindowFrameRows.
func supportsWindowFrameRange(d dialect.Dialect) bool {
	wd, ok := d.(dialect.WindowFrameDialect)

	return ok && wd.SupportsWindowFrameRange()
}

// supportsWindowFrameGroups reports whether d supports a GROUPS frame; see
// supportsWindowFrameRows.
func supportsWindowFrameGroups(d dialect.Dialect) bool {
	wd, ok := d.(dialect.WindowFrameDialect)

	return ok && wd.SupportsWindowFrameGroups()
}

// frameClause renders the `ROWS|RANGE|GROUPS BETWEEN <start> AND <end>`
// frame text for a window, or "" when mode is FrameNone (no frame clause).
// It validates the requested mode against d's WindowFrameDialect capability
// -- an unsupported mode yields a typed dialect.ErrUnsupportedByDialect --
// and validates the bounds (non-negative offsets, a start bound not after
// the end) as typed rendering errors, never a panic or invalid SQL.
func frameClause(d dialect.Dialect, mode FrameMode, start, end FrameBound) (string, error) {
	if mode == FrameNone {
		return "", nil
	}

	var supported bool

	switch mode { //nolint:exhaustive // FrameNone is handled by the early return above
	case FrameRows:
		supported = supportsWindowFrameRows(d)
	case FrameRange:
		supported = supportsWindowFrameRange(d)
	case FrameGroups:
		supported = supportsWindowFrameGroups(d)
	default:
		return "", fmt.Errorf("orm/render: unknown window frame mode %d", mode)
	}

	if !supported {
		return "", fmt.Errorf("orm/render: %w: dialect %q does not support %s window frames", dialect.ErrUnsupportedByDialect, d.Name(), mode.keyword())
	}

	startText, err := frameBoundText(start)
	if err != nil {
		return "", err
	}

	endText, err := frameBoundText(end)
	if err != nil {
		return "", err
	}

	if start.Kind == BoundUnboundedFollowing {
		return "", fmt.Errorf("orm/render: invalid window frame: start cannot be %s", startText)
	}

	if end.Kind == BoundUnboundedPreceding {
		return "", fmt.Errorf("orm/render: invalid window frame: end cannot be %s", endText)
	}

	if sr, er := frameBoundRank(start.Kind), frameBoundRank(end.Kind); sr > er {
		return "", fmt.Errorf("orm/render: invalid window frame: start %s sorts after end %s", startText, endText)
	}

	return mode.keyword() + " BETWEEN " + startText + " AND " + endText, nil
}

// OverClause is one window function's OVER (PARTITION BY ... ORDER BY ...)
// clause. Both fields are optional; an empty OverClause renders as
// `OVER ()` (the whole result set as a single window), which is valid SQL
// on every supported dialect. FrameMode/FrameStart/FrameEnd optionally add
// a `ROWS`/`RANGE`/`GROUPS BETWEEN ... AND ...` frame after the ORDER BY;
// FrameMode == FrameNone (the zero value) renders no frame clause.
type OverClause struct {
	Partition  []string
	Order      []OrderTerm
	FrameMode  FrameMode
	FrameStart FrameBound
	FrameEnd   FrameBound
}

// WindowExpr is one window expression in a WindowSelect select list.
// Exactly one of Func (a scalar window function) or Agg (an aggregate over
// a window) is set -- never both, and never neither (a zero Func with no
// Agg is a caller bug and renders nothing usable, so it is rejected with a
// rendering-time error rather than silently-wrong SQL). Column is the
// function's argument column; Value is NTile's bucket count (unused by
// every other function; LEAD/LAG use the default offset of 1). Alias is
// the output column name, always rendered.
type WindowExpr struct {
	Func   WindowFunc
	Column string
	Value  int
	Alias  string
	Over   OverClause
	Agg    *Aggregate
}

// selectExpr renders e as it appears in a SELECT list: the function call
// (or aggregate call), then its OVER clause, then `AS "alias"`. It
// continues counter for any FTS ranking terms inside the OVER ORDER BY
// clause and returns their bound arguments (an OVER clause binds nothing
// otherwise). err is non-nil for a zero WindowExpr (neither Func nor Agg
// set) or an NTile with a bucket count below 1 -- both are caller bugs
// that must not render broken SQL.
func (e WindowExpr) selectExpr(d dialect.Dialect, counter *argCounter) (string, []any, error) {
	var args []any

	var call string

	if e.Agg != nil {
		call = e.Agg.exprOnly(d)
	} else {
		switch e.Func {
		case WinNTile:
			if e.Value < 1 {
				return "", nil, fmt.Errorf("orm/render: NTile bucket count %d must be >= 1", e.Value)
			}

			call = "NTILE(" + strconv.Itoa(e.Value) + ")"
		case WinLead, WinLag:
			// No explicit offset argument: LEAD/LAG default to a 1-row
			// offset, which is the minimal supported surface (a
			// caller-specified offset is a future phase's concern).
			call = e.Func.name() + "(" + quoteColumn(d, e.Column) + ")"
		case WinNone:
			// A zero Func with no Agg is an empty window expression --
			// nothing usable to render. Rejected loudly rather than
			// falling back to `COUNT(*) OVER () OVER ()`.
			return "", nil, errors.New("orm/render: empty window expression")
		case WinRowNumber, WinRank, WinDenseRank:
			arg := ""
			if e.Column != "" {
				arg = quoteColumn(d, e.Column)
			}

			call = e.Func.name() + "(" + arg + ")"
		default:
			return "", nil, fmt.Errorf("orm/render: unsupported window function %d", e.Func)
		}
	}

	over := "OVER ("
	parts := make([]string, 0, 2)

	if len(e.Over.Partition) > 0 {
		parts = append(parts, "PARTITION BY "+joinQuoted(d, e.Over.Partition))
	}

	if len(e.Over.Order) > 0 {
		orderText, orderArgs, err := joinOrderBy(d, e.Over.Order, counter)
		if err != nil {
			return "", nil, err
		}

		parts = append(parts, "ORDER BY "+orderText)
		args = append(args, orderArgs...)
	}

	frameText, err := frameClause(d, e.Over.FrameMode, e.Over.FrameStart, e.Over.FrameEnd)
	if err != nil {
		return "", nil, err
	}

	if frameText != "" {
		parts = append(parts, frameText)
	}

	over += strings.Join(parts, " ") + ")"

	out := call + " " + over
	if e.Alias != "" {
		out += " AS " + d.QuoteIdent(e.Alias)
	}

	return out, args, nil
}

// WindowSelect renders a window-function SELECT:
//
//	SELECT <columns>, <exprs...> FROM <table> [WHERE ...] [ORDER BY ...]
//	  [LIMIT <n>] [OFFSET <n>]
//
// and its positional arguments. columns are the entity's own columns,
// emitted first in Table.Columns() order, then every window expression in
// caller order -- callers scan result rows in exactly that order (see
// the window query's Scan). A nil/empty columns list emits only
// the expressions. order/limit/offset apply to the whole result.
func WindowSelect(
	d dialect.Dialect,
	table string,
	columns []string,
	exprs []WindowExpr,
	where Node,
	order []OrderTerm,
	limit, offset int,
) (query string, args []any, err error) {
	var b strings.Builder

	b.WriteString("SELECT ")

	selectParts := make([]string, 0, len(columns)+len(exprs))

	for _, c := range columns {
		selectParts = append(selectParts, quoteColumn(d, c))
	}

	counter := &argCounter{}

	for _, e := range exprs {
		part, partArgs, exprErr := e.selectExpr(d, counter)
		if exprErr != nil {
			return "", nil, exprErr
		}

		selectParts = append(selectParts, part)
		args = append(args, partArgs...)
	}

	b.WriteString(strings.Join(selectParts, ", "))

	b.WriteString(" FROM ")
	b.WriteString(d.QuoteIdent(table))

	tailArgs, err := writeSelectTail(&b, d, where, order, limit, offset, counter)
	if err != nil {
		return "", nil, err
	}

	args = append(args, tailArgs...)

	return b.String(), args, nil
}
