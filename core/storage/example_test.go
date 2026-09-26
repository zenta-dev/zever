package storage_test

import (
	"context"

	storagelocal "github.com/zenta-dev/zever/adapters/storage/local"
	"github.com/zenta-dev/zever/core/storage"
)

// ExampleOpen opens the local-filesystem storage backend with defaults.
func ExampleOpen() {
	_ = storage.Register(storage.AdapterLocal, storagelocal.New)

	s, err := storage.Open(storage.AdapterLocal, storage.Options{})
	if err != nil {
		return
	}

	defer func() { _ = s.Close(context.Background()) }()
}
