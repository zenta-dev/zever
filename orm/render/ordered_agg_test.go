package render

import (
	"errors"
	"reflect"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/orm/dialect/sqlite"
)

// orderedAgg builds the erased render shape of an argument-last ordered
// aggregate, the way the ArrayAgg/StringAgg/GroupConcat builders erase to.
func orderedAgg(fn AggFunc, col, delim string, hasDelim bool, order ...OrderRef) Aggregate {
	return Aggregate{
		Func:         fn,
		Column:       col,
		Alias:        "agg",
		Delimiter:    delim,
		HasDelimiter: hasDelim,
		Order:        order,
	}
}

func TestOrderedAggregateRender(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		d    dialect.Dialect
		agg  Aggregate
		want string
		args []any
	}{
		{
			"postgres array_agg ordered",
			dialectFor("postgres"),
			orderedAgg(AggArrayAgg, "name", "", false, OrderRef{Column: "qty"}),
			`SELECT array_agg("name" ORDER BY "qty" ASC) AS "agg" FROM "orders"`,
			nil,
		},
		{
			"postgres string_agg bound delimiter ordered desc",
			dialectFor("postgres"),
			orderedAgg(AggStringAgg, "name", " | ", true, OrderRef{Column: "qty", Desc: true}),
			`SELECT string_agg("name", $1 ORDER BY "qty" DESC) AS "agg" FROM "orders"`,
			[]any{" | "},
		},
		{
			"postgres string_agg bound delimiter unordered",
			dialectFor("postgres"),
			orderedAgg(AggStringAgg, "name", ",", true),
			`SELECT string_agg("name", $1) AS "agg" FROM "orders"`,
			[]any{","},
		},
		{
			"sqlite group_concat bound delimiter ordered",
			dialectFor("sqlite"),
			orderedAgg(AggGroupConcat, "name", ",", true, OrderRef{Column: "qty"}),
			`SELECT group_concat("name", ? ORDER BY "qty" ASC) AS "agg" FROM "orders"`,
			[]any{","},
		},
		{
			"sqlite group_concat no delimiter ordered",
			dialectFor("sqlite"),
			orderedAgg(AggGroupConcat, "name", "", false, OrderRef{Column: "qty"}),
			`SELECT group_concat("name" ORDER BY "qty" ASC) AS "agg" FROM "orders"`,
			nil,
		},
		{
			"sqlite group_concat plain",
			dialectFor("sqlite"),
			orderedAgg(AggGroupConcat, "title", "", false),
			`SELECT group_concat("title") AS "agg" FROM "orders"`,
			nil,
		},
		{
			"sqlite group_concat hostile delimiter stays bound",
			dialectFor("sqlite"),
			orderedAgg(AggGroupConcat, "name", `a'b\`, true),
			`SELECT group_concat("name", ?) AS "agg" FROM "orders"`,
			[]any{`a'b\`},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			q, args, err := GroupedSelect(tc.d, "orders", nil, []Aggregate{tc.agg}, Node{}, HavingNode{})
			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}

			if q != tc.want {
				t.Fatalf("query = %q, want %q", q, tc.want)
			}

			if !reflect.DeepEqual(args, tc.args) {
				t.Fatalf("args = %#v, want %#v", args, tc.args)
			}
		})
	}
}

// TestOrderedAggregateDialectGates pins the per-dialect function gates:
// array_agg/string_agg are Postgres-only, group_concat is SQLite-only. A
// mismatch is a typed dialect.ErrUnsupportedByDialect, never silently-wrong
// SQL.
func TestOrderedAggregateDialectGates(t *testing.T) {
	t.Parallel()
	arrayOrdered := orderedAgg(AggArrayAgg, "name", "", false, OrderRef{Column: "qty"})
	stringOrdered := orderedAgg(AggStringAgg, "name", ",", true, OrderRef{Column: "qty"})
	groupOrdered := orderedAgg(AggGroupConcat, "name", ",", true, OrderRef{Column: "qty"})

	tests := []struct {
		name string
		d    dialect.Dialect
		agg  Aggregate
	}{
		{"array_agg/sqlite", dialectFor("sqlite"), arrayOrdered},
		{"string_agg/sqlite", dialectFor("sqlite"), stringOrdered},
		{"group_concat/postgres", dialectFor("postgres"), groupOrdered},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := GroupedSelect(tc.d, "orders", nil, []Aggregate{tc.agg}, Node{}, HavingNode{})
			if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
				t.Fatalf("err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
			}
		})
	}
}

// TestOrderedAggregateSQLiteVersionGate pins the SQLite 3.44.0 floor on the
// in-aggregate ORDER BY: below it the unordered group_concat still renders,
// but a requested order term is a typed ErrUnsupportedByDialect.
func TestOrderedAggregateSQLiteVersionGate(t *testing.T) {
	t.Parallel()
	old, err := sqlite.NewWithVersion("3.43.0")
	if err != nil {
		t.Fatalf("NewWithVersion: %v", err)
	}

	plain := orderedAgg(AggGroupConcat, "name", ",", true)
	_, _, aggErr := GroupedSelect(old, "orders", nil, []Aggregate{plain}, Node{}, HavingNode{})
	if aggErr != nil {
		t.Fatalf("unordered group_concat on 3.43 err = %v, want nil", aggErr)
	}

	ordered := orderedAgg(AggGroupConcat, "name", ",", true, OrderRef{Column: "qty"})
	_, _, aggErr = GroupedSelect(old, "orders", nil, []Aggregate{ordered}, Node{}, HavingNode{})
	if !errors.Is(aggErr, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("ordered group_concat on 3.43 err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", aggErr)
	}

	newer, err := sqlite.NewWithVersion("3.44.0")
	if err != nil {
		t.Fatalf("NewWithVersion: %v", err)
	}

	if _, _, err := GroupedSelect(newer, "orders", nil, []Aggregate{ordered}, Node{}, HavingNode{}); err != nil {
		t.Fatalf("ordered group_concat on 3.44 err = %v, want nil", err)
	}
}

// TestOrderedAggregateInvalidShapeFailsClosed proves an erased aggregate that
// carries a caller error (a bad delimiter arity or an unusable ORDER BY term)
// is rejected at render time with a typed ErrUnsupported, never rendered as
// silently-wrong SQL.
func TestOrderedAggregateInvalidShapeFailsClosed(t *testing.T) {
	t.Parallel()
	bad := orderedAgg(AggGroupConcat, "name", "", false)
	bad.Invalid = "group_concat accepts at most one delimiter"

	_, _, err := GroupedSelect(dialectFor("sqlite"), "orders", nil, []Aggregate{bad}, Node{}, HavingNode{})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("err = %v, want errors.Is(err, ErrUnsupported)", err)
	}
}
