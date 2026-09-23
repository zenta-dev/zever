package ormdrill

// ExampleDemoQueryLogger is the executable form of the query-debug-hook
// section: one captured query plus bound args.
func ExampleDemoQueryLogger() {
	runDemo(DemoQueryLogger)

	// Output:
	// == SetQueryLogger: capture one query
	//   captured: SELECT COUNT(*) FROM "orders" WHERE "amount_cents" > ? [1000]
	//   count (amount_cents > 1000) = 9
}
