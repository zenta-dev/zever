package cache

import "github.com/zenta-dev/zever/shared/ref"

// Ref returns the typed reference to the named cache service instance. It
// carries no value: the container resolves it to exactly one Cache built
// from Options.
func Ref(name string) ref.Ref[Cache, Options] {
	return ref.Named[Cache, Options]("cache", name)
}

// DefaultRef returns the typed reference to the default cache service
// instance.
func DefaultRef() ref.Ref[Cache, Options] {
	return ref.Default[Cache, Options]("cache")
}
