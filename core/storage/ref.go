package storage

import "github.com/zenta-dev/zever/shared/ref"

// Ref returns the typed reference to the named storage service instance.
// It carries no value: the container resolves it to exactly one Storage
// built from Options.
func Ref(name string) ref.Ref[Storage, Options] {
	return ref.Named[Storage, Options]("storage", name)
}

// DefaultRef returns the typed reference to the default storage service
// instance.
func DefaultRef() ref.Ref[Storage, Options] {
	return ref.Default[Storage, Options]("storage")
}
