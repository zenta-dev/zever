package ormdrill

// ExampleDemoJoinOn3 runs the INNER three-way projection join.
func ExampleDemoJoinOn3() {
	runDemo(DemoJoinOn3)

	// Output:
	// == Join3: widgets -> orders -> shipments (inner)
	//   w02 -> o02 (1000 cents) -> H002
	//   w04 -> o04 (1500 cents) -> H004
	//   w03 -> o08 (1250 cents) -> H008
	//   w05 -> o10 (1750 cents) -> H010
	//   w02 -> o12 (1000 cents) -> H012
	//   w04 -> o14 (1500 cents) -> H014
	//   6 matching rows (1 SELECT, 1 scan per row)
}

// ExampleDemoRightJoin runs the null-safe two-table RIGHT JOIN.
func ExampleDemoRightJoin() {
	runDemo(DemoRightJoin)

	// Output:
	// == RightJoin2 on SQLite: null-safe RIGHT JOIN
	//   15 rows, 0 with no matching widget (A = None)
}

// ExampleDemoJoinedUpdate runs UPDATE ... FROM scoped by the typed relation.
func ExampleDemoJoinedUpdate() {
	runDemo(DemoJoinedUpdate)

	// Output:
	// == Update.Join: UPDATE widgets SET ... FROM orders (sqlite)
	//   5 widgets affected (the 5 with orders); now priced at 1299: count = 5
}
