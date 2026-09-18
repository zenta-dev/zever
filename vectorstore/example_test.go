package vectorstore_test

import (
	"github.com/zenta-dev/zever/vectorstore"
	vectorstoresqlite "github.com/zenta-dev/zever/vectorstore/sqlite"
)

// ExampleOpen opens the sqlite vector store on a private in-memory database.
func ExampleOpen() {
	_ = vectorstore.Register(vectorstore.SQLite, vectorstoresqlite.Open)

	vs, err := vectorstore.Open(vectorstore.SQLite, vectorstore.Options{DSN: ":memory:"})
	if err != nil {
		return
	}

	defer func() { _ = vs.Close() }()
}
