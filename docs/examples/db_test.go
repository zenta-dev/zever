package examples_test

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/adapters/db/sqlite"
	"github.com/zenta-dev/zever/core/db"
)

// Example_sqliteOpenExecQuery mirrors the sqlite doc sample: open an
// in-memory database directly, create a table, insert a row, and read it
// back. There is no registry: the constructor is imported and called.
func Example_sqliteOpenExecQuery() {
	conn, err := sqlite.New(db.Options{Path: ":memory:"})
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
