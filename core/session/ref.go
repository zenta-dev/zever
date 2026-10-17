package session

import "github.com/zenta-dev/zever/shared/ref"

// Ref returns the typed reference to the named session service instance.
// It carries no value: the container resolves it to exactly one Store built
// from Options.
func Ref(name string) ref.Ref[Store, Options] {
	return ref.Named[Store, Options]("session", name)
}

// DefaultRef returns the typed reference to the default session service
// instance.
func DefaultRef() ref.Ref[Store, Options] {
	return ref.Default[Store, Options]("session")
}
