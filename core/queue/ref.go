package queue

import "github.com/zenta-dev/zever/shared/ref"

// Ref returns the typed reference to the named queue service instance. It
// carries no value: the container resolves it to exactly one Queue built
// from Options.
func Ref(name string) ref.Ref[Queue, Options] {
	return ref.Named[Queue, Options]("queue", name)
}

// DefaultRef returns the typed reference to the default queue service
// instance.
func DefaultRef() ref.Ref[Queue, Options] {
	return ref.Default[Queue, Options]("queue")
}
