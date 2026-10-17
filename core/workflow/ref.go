package workflow

import "github.com/zenta-dev/zever/shared/ref"

// Ref returns the typed reference to the named workflow service instance.
// It carries no value: the container resolves it to exactly one Workflow
// built from Options.
func Ref(name string) ref.Ref[Workflow, Options] {
	return ref.Named[Workflow, Options]("workflow", name)
}

// DefaultRef returns the typed reference to the default workflow service
// instance.
func DefaultRef() ref.Ref[Workflow, Options] {
	return ref.Default[Workflow, Options]("workflow")
}
