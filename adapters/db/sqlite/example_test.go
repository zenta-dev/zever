package sqlite_test

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/adapters/db/sqlite"
	"github.com/zenta-dev/zever/core/db"
)

// ExampleOpen opens an in-memory sqlite database and reads a row back.
func ExampleOpen() {
	if err := db.Register(db.SQLite, sqlite.New); err != nil {
		return
	}

	conn, err := db.Open(db.SQLite, db.Options{Path: ":memory:"})
	if err != nil {
		return
	}

	ctx := context.Background()

	defer func() { _ = conn.Close(ctx) }()

	if _, execErr := conn.Exec(ctx, "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)"); execErr != nil {
		return
	}

	if _, insertErr := conn.Exec(ctx, "INSERT INTO users (name) VALUES (?)", "ada"); insertErr != nil {
		return
	}

	rows, err := conn.Query(ctx, "SELECT name FROM users")
	if err != nil {
		return
	}

	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return
		}

		fmt.Println(name)
	}
	// Output: ada
}
