package lock

import "github.com/zenta-dev/zever/shared/ref"

// Ref returns the typed reference to the named lock service instance. It
// carries no value: the container resolves it to exactly one Locker built
// from Options.
func Ref(name string) ref.Ref[Locker, Options] {
	return ref.Named[Locker, Options]("lock", name)
}

// DefaultRef returns the typed reference to the default lock service
// instance.
func DefaultRef() ref.Ref[Locker, Options] {
	return ref.Default[Locker, Options]("lock")
}
