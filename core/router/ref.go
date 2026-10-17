package router

import "github.com/zenta-dev/zever/shared/ref"

// Ref returns the typed reference to the named router service instance. It
// carries no value: the container resolves it to exactly one Router built
// from Options.
func Ref(name string) ref.Ref[Router, Options] {
	return ref.Named[Router, Options]("router", name)
}

// DefaultRef returns the typed reference to the default router service
// instance.
func DefaultRef() ref.Ref[Router, Options] {
	return ref.Default[Router, Options]("router")
}
