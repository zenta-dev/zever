package ormdrill

// ExampleDemoPreload fetches every widget with its orders in exactly
// 2 queries.
func ExampleDemoPreload() {
	runDemo(DemoPreload)

	// Output:
	// == Preload: widgets with their orders (N+1-free, 2 queries)
	//   2 queries (parents all + one FK-IN fetch)
	//   w01 (gadget-01) has 3 order(s): o01 o06 o11
	//   w02 (gadget-02) has 3 order(s): o02 o07 o12
	//   w03 (gadget-03) has 3 order(s): o03 o08 o13
	//   w04 (gadget-04) has 3 order(s): o04 o09 o14
	//   w05 (gadget-05) has 3 order(s): o05 o10 o15
	//   w06 (gadget-06) has 0 order(s):
	//   w07 (gadget-07) has 0 order(s):
	//   w08 (gadget-08) has 0 order(s):
}

// ExampleDemoRetryTx commits one write through the serialization-retry
// transaction runner.
func ExampleDemoRetryTx() {
	runDemo(DemoRetryTx)

	// Output:
	// == RetryTx: a write that commits on the first attempt
	//   committed shipment s99 for order o01 (fedex F999)
	//   IsRetryable(ErrRetryable) = true (the caller-side escape hatch)
}

// ExampleDemoFirstOrErr shows the hit and the db.ErrNotFound miss of the
// error-returning single-row fetch.
func ExampleDemoFirstOrErr() {
	runDemo(DemoFirstOrErr)

	// Output:
	// == FirstOrErr: fetch-by-id, hit and db.ErrNotFound miss
	//   hit: w01 (gadget-01, 550 cents)
	//   miss: orm: Query.FirstOrErr: db: not found
	//   errors.Is(err, db.ErrNotFound) = true
}
