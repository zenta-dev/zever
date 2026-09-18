package storage_test

import (
	"context"

	"github.com/zenta-dev/zever/storage"
	storagelocal "github.com/zenta-dev/zever/storage/local"
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
