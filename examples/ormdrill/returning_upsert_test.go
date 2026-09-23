package ormdrill

// ExampleDemoMutationReturning scans post-write rows back from UPDATE and
// DELETE ... RETURNING.
func ExampleDemoMutationReturning() {
	runDemo(DemoMutationReturning)

	// Output:
	// == Round 3: UPDATE / DELETE ... RETURNING (Postgres/SQLite)
	//   UPDATE w08 -> w08 now 9999 cents (post-write values)
	//   DELETE w99 ... RETURNING -> w99 (gadget-99)
}

// ExampleDemoUpsertWhere shows the partial-index ON CONFLICT predicate and
// the conditional DO UPDATE WHERE, first false then true.
func ExampleDemoUpsertWhere() {
	runDemo(DemoUpsertWhere)

	// Output:
	// == Round 6: partial upsert WHERE (target predicate + DO UPDATE WHERE)
	//   existing discount 10: predicate >1000 false -> 10; predicate <1000 true -> 99
}
