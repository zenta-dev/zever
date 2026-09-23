package ormdrill

// ExampleDemoCursorPagination walks all 15 orders in keyset pages of 3,
// round-trips each cursor token, and shows the OffsetPage alternative.
func ExampleDemoCursorPagination() {
	runDemo(DemoCursorPagination)

	// Output:
	// == CursorKey: keyset walk over orders (created_at DESC, page size 3)
	//   page 1: o15 o14 o13
	//   next cursor AQEGb3JkZXJzCmNyZWF0ZWRfYXR0FDIwMjYtMDktMDFUMTM6MDA6MDBa; replay against OrderCols.ID rejected: orm: CursorKey.Decode: token is for column "created_at" of table "orders", not "id" of table "orders"
	//   page 2: o12 o11 o10
	//   next cursor AQEGb3JkZXJzCmNyZWF0ZWRfYXR0FDIwMjYtMDktMDFUMTA6MDA6MDBa; replay against OrderCols.ID rejected: orm: CursorKey.Decode: token is for column "created_at" of table "orders", not "id" of table "orders"
	//   page 3: o09 o08 o07
	//   next cursor AQEGb3JkZXJzCmNyZWF0ZWRfYXR0FDIwMjYtMDktMDFUMDc6MDA6MDBa; replay against OrderCols.ID rejected: orm: CursorKey.Decode: token is for column "created_at" of table "orders", not "id" of table "orders"
	//   page 4: o06 o05 o04
	//   next cursor AQEGb3JkZXJzCmNyZWF0ZWRfYXR0FDIwMjYtMDktMDFUMDQ6MDA6MDBa; replay against OrderCols.ID rejected: orm: CursorKey.Decode: token is for column "created_at" of table "orders", not "id" of table "orders"
	//   page 5: o03 o02 o01
	//   next cursor AQEGb3JkZXJzCmNyZWF0ZWRfYXR0FDIwMjYtMDktMDFUMDE6MDA6MDBa; replay against OrderCols.ID rejected: orm: CursorKey.Decode: token is for column "created_at" of table "orders", not "id" of table "orders"
	//   OffsetPage(page 2, size 3) = [o12 o11 o10]
}
