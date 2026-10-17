package crypto

import "github.com/zenta-dev/zever/shared/ref"

// Ref returns the typed reference to the named crypto service instance. It
// carries no value: the container resolves it to exactly one Crypto built
// from Options.
func Ref(name string) ref.Ref[Crypto, Options] {
	return ref.Named[Crypto, Options]("crypto", name)
}

// DefaultRef returns the typed reference to the default crypto service
// instance.
func DefaultRef() ref.Ref[Crypto, Options] {
	return ref.Default[Crypto, Options]("crypto")
}
