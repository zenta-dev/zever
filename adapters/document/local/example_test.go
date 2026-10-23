package local_test

import (
	"github.com/zenta-dev/zever/adapters/document/local"
	"github.com/zenta-dev/zever/core/document"
)

// Example opens the local renderer and releases it without rendering.
func Example() {
	doc, err := local.New(document.Options{})
	if err != nil {
		return
	}

	_ = doc.Close()
}
