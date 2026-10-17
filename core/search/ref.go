package search

import "github.com/zenta-dev/zever/shared/ref"

// Ref returns the typed reference to the named search service instance. It
// carries no value: the container resolves it to exactly one Search built
// from Options.
func Ref(name string) ref.Ref[Search, Options] {
	return ref.Named[Search, Options]("search", name)
}

// DefaultRef returns the typed reference to the default search service
// instance.
func DefaultRef() ref.Ref[Search, Options] {
	return ref.Default[Search, Options]("search")
}
