package scheduler

import "github.com/zenta-dev/zever/shared/ref"

// Ref returns the typed reference to the named scheduler service instance.
// It carries no value: the container resolves it to exactly one Scheduler
// built from Options.
func Ref(name string) ref.Ref[Scheduler, Options] {
	return ref.Named[Scheduler, Options]("scheduler", name)
}

// DefaultRef returns the typed reference to the default scheduler service
// instance.
func DefaultRef() ref.Ref[Scheduler, Options] {
	return ref.Default[Scheduler, Options]("scheduler")
}
