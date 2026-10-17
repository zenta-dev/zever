package cdn

import "github.com/zenta-dev/zever/shared/ref"

// Ref returns the typed reference to the named cdn service instance. It
// carries no value: the container resolves it to exactly one CDN built
// from Options.
func Ref(name string) ref.Ref[CDN, Options] {
	return ref.Named[CDN, Options]("cdn", name)
}

// DefaultRef returns the typed reference to the default cdn service
// instance.
func DefaultRef() ref.Ref[CDN, Options] {
	return ref.Default[CDN, Options]("cdn")
}
