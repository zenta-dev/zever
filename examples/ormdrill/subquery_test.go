package ormdrill

// ExampleDemoSubqueryPredicates counts InSub / NotInSub / Exists /
// NotExists / EqScalar matches.
func ExampleDemoSubqueryPredicates() {
	runDemo(DemoSubqueryPredicates)

	// Output:
	// == Round 3: subquery predicates (InSub / NotInSub / Exists / EqScalar)
	//   widgets with orders (InSub)        = 5
	//   widgets without orders (NotInSub)  = 3
	//   orders with a shipment (Exists)    = 7
	//   widgets with no orders (NotExists) = 3
	//   orders at o01's amount (EqScalar)  = 3
}

// ExampleDemoCorrelatedOuterInJoin places a correlated EXISTS subquery in a
// projecting join's A-side Where.
func ExampleDemoCorrelatedOuterInJoin() {
	runDemo(DemoCorrelatedOuterInJoin)

	// Output:
	// == Round 7: correlated outer ref inside a projecting join
	//   7 joined rows across 5 distinct widgets whose order shipped
}

// ExampleDemoTupleIn counts orders whose (widget_id, amount_cents) pair
// matches the reference order's pair.
func ExampleDemoTupleIn() {
	runDemo(DemoTupleIn)

	// Output:
	// == Round 6: multi-column tuple row-value predicate
	//   (widget_id, amount_cents) matching o01's pair = 3
}
