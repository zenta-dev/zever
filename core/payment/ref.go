package payment

import "github.com/zenta-dev/zever/shared/ref"

// Ref returns the typed reference to the named payment service instance.
// It carries no value: the container resolves it to exactly one Payment
// built from Options.
func Ref(name string) ref.Ref[Payment, Options] {
	return ref.Named[Payment, Options]("payment", name)
}

// DefaultRef returns the typed reference to the default payment service
// instance.
func DefaultRef() ref.Ref[Payment, Options] {
	return ref.Default[Payment, Options]("payment")
}
