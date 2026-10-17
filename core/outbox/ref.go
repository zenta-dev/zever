package outbox

import "github.com/zenta-dev/zever/shared/ref"

// Ref returns the typed reference to the named outbox service instance. It
// carries no value: the container resolves it to exactly one Store built
// from Options.
func Ref(name string) ref.Ref[Store, Options] {
	return ref.Named[Store, Options]("outbox", name)
}

// DefaultRef returns the typed reference to the default outbox service
// instance.
func DefaultRef() ref.Ref[Store, Options] {
	return ref.Default[Store, Options]("outbox")
}
