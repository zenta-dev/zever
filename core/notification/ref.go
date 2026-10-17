package notification

import "github.com/zenta-dev/zever/shared/ref"

// Ref returns the typed reference to the named notification service
// instance. It carries no value: the container resolves it to exactly one
// Notifier built from Options.
func Ref(name string) ref.Ref[Notifier, Options] {
	return ref.Named[Notifier, Options]("notification", name)
}

// DefaultRef returns the typed reference to the default notification
// service instance.
func DefaultRef() ref.Ref[Notifier, Options] {
	return ref.Default[Notifier, Options]("notification")
}
