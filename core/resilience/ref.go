package resilience

import "github.com/zenta-dev/zever/shared/ref"

// Ref returns the typed reference to the named resilience service
// instance. It carries no value: the container resolves it to exactly one
// Manager built from Options.
func Ref(name string) ref.Ref[Manager, Options] {
	return ref.Named[Manager, Options]("resilience", name)
}

// DefaultRef returns the typed reference to the default resilience
// service instance.
func DefaultRef() ref.Ref[Manager, Options] {
	return ref.Default[Manager, Options]("resilience")
}
