package media

import "github.com/zenta-dev/zever/shared/ref"

// Ref returns the typed reference to the named media service instance. It
// carries no value: the container resolves it to exactly one Media built
// from Options.
func Ref(name string) ref.Ref[Media, Options] {
	return ref.Named[Media, Options]("media", name)
}

// DefaultRef returns the typed reference to the default media service
// instance.
func DefaultRef() ref.Ref[Media, Options] {
	return ref.Default[Media, Options]("media")
}
