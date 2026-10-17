package eventbus

import "github.com/zenta-dev/zever/shared/ref"

// Ref returns the typed reference to the named eventbus service instance.
// It carries no value: the container resolves it to exactly one EventBus
// built from Options.
func Ref(name string) ref.Ref[EventBus, Options] {
	return ref.Named[EventBus, Options]("eventbus", name)
}

// DefaultRef returns the typed reference to the default eventbus service
// instance.
func DefaultRef() ref.Ref[EventBus, Options] {
	return ref.Default[EventBus, Options]("eventbus")
}
