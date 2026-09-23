package ormdrill

// ExampleDemoScalarExpr counts matches for each typed scalar-expression
// predicate.
func ExampleDemoScalarExpr() {
	runDemo(DemoScalarExpr)

	// Output:
	// == Round 5: typed scalar expressions (COALESCE/LOWER/UPPER/TRIM/LENGTH/NULLIF/CASE)
	//   LOWER(name) = gadget-01                = 1
	//   UPPER(name) = GADGET-02                = 1
	//   TRIM(name) = gadget-03                 = 1
	//   LENGTH(name) = 9                       = 8
	//   NULLIF(name,'gadget-01') IS NULL       = 1
	//   COALESCE(note,'n/a') = n/a             = 4
	//   CASE price >= 800 -> premium           = 3
}

// ExampleDemoProjection projects aliased scalar expressions and a correlated
// scalar subquery instead of entity columns.
func ExampleDemoProjection() {
	runDemo(DemoProjection)

	// Output:
	// == Round 8: expression projection + aliases
	//   projected 8 widgets; first: id=w01 name=gadget-01 note=(none) tier=standard
	//   First(ok=true): widget w01 largest order amount = 750
}

// ExampleDemoNullsOrdering orders the nullable note column NULLs-first and
// NULLs-last.
func ExampleDemoNullsOrdering() {
	runDemo(DemoNullsOrdering)

	// Output:
	// == Round 6: ORDER BY ... NULLS FIRST/LAST (native on SQLite)
	//   NULLS FIRST: w01:NULL w03:NULL w05:NULL w07:NULL w02:batch-02 w04:batch-04 w06:batch-06 w08:batch-08
	//   NULLS LAST:  w02:batch-02 w04:batch-04 w06:batch-06 w08:batch-08 w01:NULL w03:NULL w05:NULL w07:NULL
}
