package log

import "github.com/zenta-dev/zever/shared/ref"

// Ref returns the typed reference to the named log service instance. It
// carries no value: the container resolves it to exactly one Logger built
// from Options.
func Ref(name string) ref.Ref[Logger, Options] {
	return ref.Named[Logger, Options]("log", name)
}

// DefaultRef returns the typed reference to the default log service
// instance.
func DefaultRef() ref.Ref[Logger, Options] {
	return ref.Default[Logger, Options]("log")
}
