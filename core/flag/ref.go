package flag

import "github.com/zenta-dev/zever/shared/ref"

// Ref returns the typed reference to the named flag service instance. It
// carries no value: the container resolves it to exactly one Flag built
// from Options.
func Ref(name string) ref.Ref[Flag, Options] {
	return ref.Named[Flag, Options]("flag", name)
}

// DefaultRef returns the typed reference to the default flag service
// instance.
func DefaultRef() ref.Ref[Flag, Options] {
	return ref.Default[Flag, Options]("flag")
}
