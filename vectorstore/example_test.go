package vectorstore_test

import (
	"context"

	"github.com/zenta-dev/zever/vectorstore"
	vectorstoresqlite "github.com/zenta-dev/zever/vectorstore/sqlite"
)

// ExampleOpen opens the sqlite vector store on a private in-memory database.
func ExampleOpen() {
	_ = vectorstore.Register(vectorstore.SQLite, vectorstoresqlite.New)

	vs, err := vectorstore.Open(vectorstore.SQLite, vectorstore.Options{DSN: ":memory:"})
	if err != nil {
		return
	}

	defer func() { _ = vs.Close() }()
}

// ExampleVectorStore_UpsertBatch upserts several vectors in one call instead
// of one round trip per vector.
func ExampleVectorStore_UpsertBatch() {
	_ = vectorstore.Register(vectorstore.SQLite, vectorstoresqlite.New)

	vs, err := vectorstore.Open(vectorstore.SQLite, vectorstore.Options{DSN: ":memory:"})
	if err != nil {
		return
	}

	defer func() { _ = vs.Close() }()

	err = vs.UpsertBatch(context.Background(), []vectorstore.Vector{
		{ID: "doc-1", Embedding: []float32{0.1, 0.2, 0.3}},
		{ID: "doc-2", Embedding: []float32{0.4, 0.5, 0.6}},
	})
	if err != nil {
		return
	}
}
