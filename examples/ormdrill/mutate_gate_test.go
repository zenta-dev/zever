package ormdrill

// ExampleDemoInsertSelectDistinct copies filtered widgets into an archive
// table, then shows DISTINCT removing one deliberate duplicate.
func ExampleDemoInsertSelectDistinct() {
	runDemo(DemoInsertSelectDistinct)

	// Output:
	// == Round 5: INSERT ... SELECT + Query.Distinct
	//   INSERT ... SELECT copied the 4 widgets priced > 700
	//   archive rows = 5, SELECT DISTINCT rows = 4
}

// ExampleDemoLockingGates asserts the typed rejections for unsupported and
// invalid row-lock combinations on SQLite.
func ExampleDemoLockingGates() {
	runDemo(DemoLockingGates)

	// Output:
	// == Round 5: row locking and DISTINCT (typed gates on SQLite)
	//   ForUpdate                      -> rejected: orm: orm/dialect: unsupported by dialect: dialect "sqlite" does not support FOR UPDATE
	//   Distinct + ForUpdate           -> rejected: orm: Query: orm: DISTINCT cannot be combined with a row lock (FOR UPDATE/FOR SHARE)
	//   NoWait without a lock          -> rejected: orm: Query: orm: NOWAIT and SKIP LOCKED require FOR UPDATE or FOR SHARE
	//   ForShare + SkipLocked          -> rejected: orm: orm/dialect: unsupported by dialect: dialect "sqlite" does not support FOR SHARE
}

// ExampleDemoDistinctOnLocksTablesample asserts the Postgres-only SELECT
// modifiers are rejected on SQLite with the typed capability error.
func ExampleDemoDistinctOnLocksTablesample() {
	runDemo(DemoDistinctOnLocksTablesample)

	// Output:
	// == Round 8: DISTINCT ON / extended locks / TABLESAMPLE (Postgres-only)
	//   DISTINCT ON                    -> rejected: orm: orm/dialect: unsupported by dialect: dialect "sqlite" does not support DISTINCT ON
	//   FOR NO KEY UPDATE              -> rejected: orm: orm/dialect: unsupported by dialect: dialect "sqlite" does not support lock mode 3
	//   FOR KEY SHARE                  -> rejected: orm: orm/dialect: unsupported by dialect: dialect "sqlite" does not support lock mode 4
	//   FOR UPDATE OF                  -> rejected: orm: orm/dialect: unsupported by dialect: dialect "sqlite" does not support FOR UPDATE
	//   TABLESAMPLE                    -> rejected: orm: Query.Stream: orm/render: orm/dialect: unsupported by dialect: dialect "sqlite" does not support TABLESAMPLE
}

// ExampleDemoMutationOrderGate asserts single-table mutation ORDER BY/LIMIT
// is rejected outside MySQL.
func ExampleDemoMutationOrderGate() {
	runDemo(DemoMutationOrderGate)

	// Output:
	// == Round 4: mutation ORDER BY / LIMIT is MySQL-only
	//   UPDATE ... ORDER BY/LIMIT      -> rejected: orm: orm/dialect: unsupported by dialect: dialect "sqlite" does not support ORDER BY/LIMIT in UPDATE
	//   DELETE ... ORDER BY/LIMIT      -> rejected: orm: orm/dialect: unsupported by dialect: dialect "sqlite" does not support ORDER BY/LIMIT in DELETE
}
