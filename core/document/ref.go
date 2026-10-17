package document

import "github.com/zenta-dev/zever/shared/ref"

// Ref returns the typed reference to the named document service instance.
// It carries no value: the container resolves it to exactly one Document
// built from Options.
func Ref(name string) ref.Ref[Document, Options] {
	return ref.Named[Document, Options]("document", name)
}

// DefaultRef returns the typed reference to the default document service
// instance.
func DefaultRef() ref.Ref[Document, Options] {
	return ref.Default[Document, Options]("document")
}
