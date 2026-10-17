package ai

import "github.com/zenta-dev/zever/shared/ref"

// Ref returns the typed reference to the named ai service instance. It
// carries no value: the container resolves it to exactly one AI built
// from Options.
func Ref(name string) ref.Ref[AI, Options] {
	return ref.Named[AI, Options]("ai", name)
}

// DefaultRef returns the typed reference to the default ai service
// instance.
func DefaultRef() ref.Ref[AI, Options] {
	return ref.Default[AI, Options]("ai")
}
