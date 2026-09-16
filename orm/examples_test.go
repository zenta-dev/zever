package orm

import (
	"context"
	"fmt"
)

// ExampleColumn_Eq builds a Column = v predicate.
func ExampleColumn_Eq() {
	pred := widgetID.Eq("w1")
	fmt.Println(pred.IsSet())
	// Output: true
}

// ExampleQuery_Where builds up a filtered Query without mutating the base
// Query it branched from.
func ExampleQuery_Where() {
	base := From(widgets)
	filtered := base.Where(widgetName.Eq("Beta"))

	fmt.Println(base.where.IsSet(), filtered.where.IsSet())
	// Output: false true
}

// ExampleQuery_All runs a Query against a db.DB and scans every matching
// row via the entity's codegen'd Scan method.
func ExampleQuery_All() {
	ctx := context.Background()
	conn, err := openExampleDB(ctx)
	if err != nil {
		fmt.Println(err)

		return
	}
	defer func() { _ = conn.Close(ctx) }()

	rows, err := From(widgets).OrderBy(widgetID.Asc()).All(ctx, conn)
	if err != nil {
		fmt.Println(err)

		return
	}

	for _, r := range rows {
		fmt.Println(r.ID, r.Name)
	}
	// Output:
	// w1 Alpha
	// w2 Beta
	// w3 Gamma
}

// ExampleOption_Get reads back the value/presence pair an Option holds.
func ExampleOption_Get() {
	present := Some(42)
	v, ok := present.Get()
	fmt.Println(v, ok)

	absent := None[int]()
	v2, ok2 := absent.Get()
	fmt.Println(v2, ok2)
	// Output:
	// 42 true
	// 0 false
}
