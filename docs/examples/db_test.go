package examples_test

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/adapters/db/sqlite"
	"github.com/zenta-dev/zever/core/db"
)

// ExampleDB_openExecQuery mirrors the sqlite doc sample: register the
// driver, open an in-memory database, create a table, insert a row, and
// read it back.
func Example_dbOpenExecQuery() {
	_ = db.Register(db.SQLite, sqlite.New)

	conn, err := db.Open(db.SQLite, db.Options{Path: ":memory:"})
	if err != nil {
		fmt.Println("open error")
		return
	}
	defer func() { _ = conn.Close(context.Background()) }()

	ctx := context.Background()
	if _, err := conn.Exec(ctx, "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)"); err != nil {
		fmt.Println("create error")
		return
	}
	if _, err := conn.Exec(ctx, "INSERT INTO users (name) VALUES (?)", "ada"); err != nil {
		fmt.Println("insert error")
		return
	}
	rows, err := conn.Query(ctx, "SELECT name FROM users")
	if err != nil {
		fmt.Println("query error")
		return
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			fmt.Println("scan error")
			return
		}
		fmt.Println(name)
	}
	// Output: ada
}
