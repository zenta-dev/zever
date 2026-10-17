package observability

import "github.com/zenta-dev/zever/shared/ref"

// Ref returns the typed reference to the named observability service
// instance. It carries no value: the container resolves it to exactly one
// Provider built from Options.
func Ref(name string) ref.Ref[Provider, Options] {
	return ref.Named[Provider, Options]("observability", name)
}

// DefaultRef returns the typed reference to the default observability
// service instance.
func DefaultRef() ref.Ref[Provider, Options] {
	return ref.Default[Provider, Options]("observability")
}
