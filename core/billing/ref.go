package billing

import "github.com/zenta-dev/zever/shared/ref"

// Ref returns the typed reference to the named billing service instance.
// It carries no value: the container resolves it to exactly one Billing
// built from Options.
func Ref(name string) ref.Ref[Billing, Options] {
	return ref.Named[Billing, Options]("billing", name)
}

// DefaultRef returns the typed reference to the default billing service
// instance.
func DefaultRef() ref.Ref[Billing, Options] {
	return ref.Default[Billing, Options]("billing")
}
