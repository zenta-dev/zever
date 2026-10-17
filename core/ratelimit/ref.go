package ratelimit

import "github.com/zenta-dev/zever/shared/ref"

// Ref returns the typed reference to the named ratelimit service instance.
// It carries no value: the container resolves it to exactly one Limiter
// built from Options.
func Ref(name string) ref.Ref[Limiter, Options] {
	return ref.Named[Limiter, Options]("ratelimit", name)
}

// DefaultRef returns the typed reference to the default ratelimit service
// instance.
func DefaultRef() ref.Ref[Limiter, Options] {
	return ref.Default[Limiter, Options]("ratelimit")
}
