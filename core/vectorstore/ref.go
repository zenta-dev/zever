package vectorstore

import "github.com/zenta-dev/zever/shared/ref"

// Ref returns the typed reference to the named vectorstore service
// instance. It carries no value: the container resolves it to exactly one
// VectorStore built from Options.
func Ref(name string) ref.Ref[VectorStore, Options] {
	return ref.Named[VectorStore, Options]("vectorstore", name)
}

// DefaultRef returns the typed reference to the default vectorstore
// service instance.
func DefaultRef() ref.Ref[VectorStore, Options] {
	return ref.Default[VectorStore, Options]("vectorstore")
}
