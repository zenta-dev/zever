package document_test

import (
	"github.com/zenta-dev/zever/document"
	"github.com/zenta-dev/zever/document/local"
)

// ExampleOpen opens the local renderer and releases it without rendering.
func ExampleOpen() {
	if err := document.Register(document.Local, local.New); err != nil {
		return
	}

	doc, err := document.Open(document.Local, document.Options{})
	if err != nil {
		return
	}

	_ = doc.Close()
}
