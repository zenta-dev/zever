package render

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/orm/dialect/postgres"
	"github.com/zenta-dev/zever/orm/dialect/sqlite"
)

func TestWindowSelect(t *testing.T) {
	t.Parallel()
	t.Run("scalar window functions", func(t *testing.T) {
		t.Parallel()
		exprs := []WindowExpr{
			{Func: WinRowNumber, Alias: "row_number", Over: OverClause{Order: []OrderTerm{{Column: "quantity", Desc: true}}}},
			{Func: WinRank, Alias: "rank", Over: OverClause{}},
			{Func: WinDenseRank, Alias: "dense_rank", Over: OverClause{}},
			{Func: WinLead, Column: "name", Alias: "lead_name", Over: OverClause{Order: []OrderTerm{{Column: "quantity"}}}},
			{Func: WinLag, Column: "name", Alias: "lag_name", Over: OverClause{Order: []OrderTerm{{Column: "quantity"}}}},
			{Func: WinNTile, Value: 3, Alias: "ntile", Over: OverClause{Order: []OrderTerm{{Column: "quantity"}}}},
		}

		q, args, err := WindowSelect(sqlite.New(), "widgets", []string{"id", "name"}, exprs, Node{}, nil, 0, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `SELECT "id", "name", ROW_NUMBER() OVER (ORDER BY "quantity" DESC) AS "row_number", RANK() OVER () AS "rank", DENSE_RANK() OVER () AS "dense_rank", LEAD("name") OVER (ORDER BY "quantity" ASC) AS "lead_name", LAG("name") OVER (ORDER BY "quantity" ASC) AS "lag_name", NTILE(3) OVER (ORDER BY "quantity" ASC) AS "ntile" FROM "widgets"`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if len(args) != 0 {
			t.Fatalf("args = %#v, want none", args)
		}
	})

	t.Run("partition by and where args", func(t *testing.T) {
		expr := WindowExpr{
			Func: WinRowNumber, Alias: "rn",
			Over: OverClause{Partition: []string{"category"}, Order: []OrderTerm{{Column: "quantity", Desc: true}}},
		}

		where := Node{Kind: KindBinary, Op: OpEq, Column: "category", Value: "fruit"}

		q, args, err := WindowSelect(sqlite.New(), "widgets", []string{"id"}, []WindowExpr{expr}, where, nil, 0, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `SELECT "id", ROW_NUMBER() OVER (PARTITION BY "category" ORDER BY "quantity" DESC) AS "rn" FROM "widgets" WHERE "category" = ?`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{"fruit"}) {
			t.Fatalf("args = %#v, want [fruit]", args)
		}
	})

	t.Run("aggregate over window", func(t *testing.T) {
		exprs := []WindowExpr{
			{Agg: &Aggregate{Func: AggSum, Column: "quantity"}, Alias: "sum_quantity", Over: OverClause{}},
			{Agg: &Aggregate{Func: AggCount}, Alias: "count", Over: OverClause{}},
		}

		q, _, err := WindowSelect(sqlite.New(), "widgets", nil, exprs, Node{}, nil, 0, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `SELECT SUM("quantity") OVER () AS "sum_quantity", COUNT(*) OVER () AS "count" FROM "widgets"`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}
	})

	t.Run("postgres limit offset numbering follows where args", func(t *testing.T) {
		expr := WindowExpr{Func: WinRowNumber, Alias: "rn", Over: OverClause{}}

		where := Node{Kind: KindBinary, Op: OpEq, Column: "category", Value: "fruit"}

		q, args, err := WindowSelect(postgres.New(), "widgets", []string{"id"}, []WindowExpr{expr}, where, nil, 5, 10)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `SELECT "id", ROW_NUMBER() OVER () AS "rn" FROM "widgets" WHERE "category" = $1 LIMIT $2 OFFSET $3`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{"fruit", 5, 10}) {
			t.Fatalf("args = %#v, want [fruit 5 10]", args)
		}
	})

	t.Run("zero window expr is a rendering error, not COUNT(*) OVER () OVER ()", func(t *testing.T) {
		_, _, err := WindowSelect(sqlite.New(), "widgets", nil, []WindowExpr{{}}, Node{}, nil, 0, 0)
		if err == nil {
			t.Fatalf("err = nil, want an empty-window-expression error")
		}

		if !strings.Contains(err.Error(), "empty window expression") {
			t.Fatalf("err = %v, want it to mention an empty window expression", err)
		}
	})

	t.Run("NTile with a bucket count below 1 is a rendering error", func(t *testing.T) {
		exprs := []WindowExpr{{Func: WinNTile, Value: 0, Alias: "ntile"}}

		_, _, err := WindowSelect(sqlite.New(), "widgets", nil, exprs, Node{}, nil, 0, 0)
		if err == nil {
			t.Fatalf("err = nil, want an NTile bucket-count error")
		}

		if !strings.Contains(err.Error(), "NTile") {
			t.Fatalf("err = %v, want it to mention NTile", err)
		}
	})
}

// TestWindowSelectFrames pins the ROWS/RANGE/GROUPS frame clause rendering
// for each SQL frame mode and a representative bound pair, appended after
// the OVER clause's PARTITION BY / ORDER BY parts.
func TestWindowSelectFrames(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		mode       FrameMode
		start, end FrameBound
		wantFrame  string
	}{
		{"rows running total", FrameRows, UnboundedPreceding(), CurrentRow(), "ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW"},
		{"range moving window", FrameRange, Preceding(2), Following(2), "RANGE BETWEEN 2 PRECEDING AND 2 FOLLOWING"},
		{"groups to end", FrameGroups, CurrentRow(), UnboundedFollowing(), "GROUPS BETWEEN CURRENT ROW AND UNBOUNDED FOLLOWING"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			expr := WindowExpr{
				Agg:   &Aggregate{Func: AggSum, Column: "quantity"},
				Alias: "sum_quantity",
				Over: OverClause{
					Order:     []OrderTerm{{Column: "quantity"}},
					FrameMode: tc.mode, FrameStart: tc.start, FrameEnd: tc.end,
				},
			}

			q, _, err := WindowSelect(sqlite.New(), "widgets", nil, []WindowExpr{expr}, Node{}, nil, 0, 0)
			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}

			want := `SELECT SUM("quantity") OVER (ORDER BY "quantity" ASC ` + tc.wantFrame + `) AS "sum_quantity" FROM "widgets"`
			if q != want {
				t.Fatalf("query = %q, want %q", q, want)
			}
		})
	}
}

// TestWindowSelectFrameErrors proves malformed frame bounds surface as a
// typed rendering error rather than a panic or invalid SQL: negative
// PRECEDING/FOLLOWING offsets, UNBOUNDED FOLLOWING as a start, UNBOUNDED
// PRECEDING as an end, a start bound that sorts after the end, and an
// out-of-range bound kind.
func TestWindowSelectFrameErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		start, end FrameBound
	}{
		{"negative preceding", Preceding(-1), CurrentRow()},
		{"negative following", CurrentRow(), Following(-1)},
		{"unbounded following start", UnboundedFollowing(), UnboundedFollowing()},
		{"unbounded preceding end", UnboundedPreceding(), UnboundedPreceding()},
		{"start after end", Following(1), Preceding(1)},
		{"unknown bound kind", FrameBound{Kind: FrameBoundKind(99)}, CurrentRow()},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			expr := WindowExpr{
				Func: WinRowNumber, Alias: "rn",
				Over: OverClause{
					Order:     []OrderTerm{{Column: "quantity"}},
					FrameMode: FrameRows, FrameStart: tc.start, FrameEnd: tc.end,
				},
			}

			_, _, err := WindowSelect(sqlite.New(), "widgets", nil, []WindowExpr{expr}, Node{}, nil, 0, 0)
			if err == nil {
				t.Fatalf("err = nil, want a malformed-frame error")
			}
		})
	}
}

// baseOnlyDialect implements only dialect.Dialect -- no WindowFrameDialect
// -- so every frame capability assertion fails on it.
type baseOnlyDialect struct{}

func (baseOnlyDialect) Name() string               { return "base-only" }
func (baseOnlyDialect) Placeholder(int) string     { return "?" }
func (baseOnlyDialect) QuoteIdent(s string) string { return `"` + s + `"` }

// TestWindowSelectFrameCapability pins the capability truth: Postgres
// supports all three frame modes, SQLite gates all three at 3.28.0, and a
// dialect with no WindowFrameDialect at all rejects every frame.
func TestWindowSelectFrameCapability(t *testing.T) {
	t.Parallel()
	frame := func(mode FrameMode) WindowExpr {
		return WindowExpr{
			Agg: &Aggregate{Func: AggSum, Column: "quantity"}, Alias: "s",
			Over: OverClause{
				Order:     []OrderTerm{{Column: "quantity"}},
				FrameMode: mode, FrameStart: UnboundedPreceding(), FrameEnd: CurrentRow(),
			},
		}
	}

	for _, mode := range []FrameMode{FrameRows, FrameRange, FrameGroups} {
		if _, _, err := WindowSelect(postgres.New(), "widgets", nil, []WindowExpr{frame(mode)}, Node{}, nil, 0, 0); err != nil {
			t.Fatalf("postgres frame mode %d err = %v, want nil", mode, err)
		}
	}

	if _, _, err := WindowSelect(baseOnlyDialect{}, "widgets", nil, []WindowExpr{frame(FrameRows)}, Node{}, nil, 0, 0); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("base-only ROWS err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}

	old, err := sqlite.NewWithVersion("3.27.2")
	if err != nil {
		t.Fatalf("sqlite.NewWithVersion: %v", err)
	}

	_, _, winErr := WindowSelect(old, "widgets", nil, []WindowExpr{frame(FrameGroups)}, Node{}, nil, 0, 0)
	if !errors.Is(winErr, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("sqlite 3.27 GROUPS err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", winErr)
	}

	newer, err := sqlite.NewWithVersion("3.28.0")
	if err != nil {
		t.Fatalf("sqlite.NewWithVersion: %v", err)
	}

	if _, _, err := WindowSelect(newer, "widgets", nil, []WindowExpr{frame(FrameGroups)}, Node{}, nil, 0, 0); err != nil {
		t.Fatalf("sqlite 3.28 GROUPS err = %v, want nil", err)
	}
}

func TestWindowFuncAndFrameModeNames(t *testing.T) {
	t.Parallel()

	if got := WinNone.name(); got != "" {
		t.Fatalf("WinNone.name() = %q, want empty", got)
	}

	if got := WinNTile.name(); got != "NTILE" {
		t.Fatalf("WinNTile.name() = %q, want NTILE", got)
	}

	if got := WindowFunc(99).name(); got != "" {
		t.Fatalf("WindowFunc(99).name() = %q, want empty", got)
	}

	if got := FrameNone.keyword(); got != "" {
		t.Fatalf("FrameNone.keyword() = %q, want empty", got)
	}

	if got := FrameMode(99).keyword(); got != "" {
		t.Fatalf("FrameMode(99).keyword() = %q, want empty", got)
	}

	if got := frameBoundRank(FrameBoundKind(99)); got != -1 {
		t.Fatalf("frameBoundRank(99) = %d, want -1", got)
	}
}

func TestWindowSelectFrameAndFuncErrors(t *testing.T) {
	t.Parallel()

	t.Run("unknown frame mode rejected", func(t *testing.T) {
		t.Parallel()

		expr := WindowExpr{Func: WinRowNumber, Alias: "rn",
			Over: OverClause{FrameMode: FrameMode(99), FrameStart: UnboundedPreceding(), FrameEnd: CurrentRow()}}

		_, _, err := WindowSelect(postgres.New(), "widgets", nil, []WindowExpr{expr}, Node{}, nil, 0, 0)
		if err == nil {
			t.Fatal("err = nil, want an unknown-mode error")
		}
	})

	t.Run("unknown window function rejected", func(t *testing.T) {
		t.Parallel()

		expr := WindowExpr{Func: WindowFunc(99), Alias: "x", Over: OverClause{}}

		_, _, err := WindowSelect(postgres.New(), "widgets", nil, []WindowExpr{expr}, Node{}, nil, 0, 0)
		if err == nil {
			t.Fatal("err = nil, want an unknown-function error")
		}
	})

	t.Run("row number with column argument", func(t *testing.T) {
		t.Parallel()

		expr := WindowExpr{Func: WinRowNumber, Column: "id", Alias: "rn", Over: OverClause{}}

		q, _, err := WindowSelect(postgres.New(), "widgets", nil, []WindowExpr{expr}, Node{}, nil, 0, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `SELECT ROW_NUMBER("id") OVER () AS "rn" FROM "widgets"`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}
	})

	t.Run("over order error propagates", func(t *testing.T) {
		t.Parallel()

		expr := WindowExpr{Func: WinRowNumber, Alias: "rn",
			Over: OverClause{Order: []OrderTerm{{Func: &FuncExpr{}}}}}

		_, _, err := WindowSelect(postgres.New(), "widgets", nil, []WindowExpr{expr}, Node{}, nil, 0, 0)
		if err == nil {
			t.Fatal("err = nil, want the OVER ORDER BY error")
		}
	})

	t.Run("where error propagates", func(t *testing.T) {
		t.Parallel()

		bad := Node{Kind: KindBinary, Column: "id", Op: OpEqAny, Value: "w1"}

		_, _, err := WindowSelect(postgres.New(), "widgets", []string{"id"}, nil, bad, nil, 0, 0)
		if err == nil {
			t.Fatal("err = nil, want the WHERE error")
		}
	})
}
