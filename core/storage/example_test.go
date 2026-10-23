package storage_test

import (
	"context"

	storagelocal "github.com/zenta-dev/zever/adapters/storage/local"
	"github.com/zenta-dev/zever/core/storage"
)

// Example opens the local-filesystem storage backend with defaults.
func Example() {
	s, err := storagelocal.New(storage.Options{})
	if err != nil {
		return
	}

	defer func() { _ = s.Close(context.Background()) }()
}
