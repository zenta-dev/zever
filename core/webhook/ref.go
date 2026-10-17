package webhook

import "github.com/zenta-dev/zever/shared/ref"

// Ref returns the typed reference to the named webhook service instance.
// It carries no value: the container resolves it to exactly one Webhook
// built from Options.
func Ref(name string) ref.Ref[Webhook, Options] {
	return ref.Named[Webhook, Options]("webhook", name)
}

// DefaultRef returns the typed reference to the default webhook service
// instance.
func DefaultRef() ref.Ref[Webhook, Options] {
	return ref.Default[Webhook, Options]("webhook")
}
