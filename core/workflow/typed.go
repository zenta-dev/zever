package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// StepFuncOf is a typed workflow step: input of type In in, output of type
// Out or error out. In and Out must be JSON-marshalable; the typed-to-untyped
// bridge (see NewTypedStep) converts across step boundaries with encoding/json
// so heterogeneous steps can share one saga definition.
type StepFuncOf[In, Out any] func(ctx context.Context, input In) (Out, error)

// TypedStep is one saga step with typed forward and compensating actions.
// The zero value is invalid; build values with NewTypedStep or
// NewTypedStepWithoutCompensate. Convert to the engine's untyped shape with
// AsSagaStep, or register a batch with a TypedSagaRegistrar.
type TypedStep struct {
	name       string
	execute    StepFunc
	compensate StepFunc
	pivot      bool
}

// NewTypedStep returns the TypedStep for the named step: execute runs the
// forward action, compensate undoes it, pivot marks the step after which
// failures roll forward instead of compensating. The typed functions are
// bridged to the engine's untyped StepFunc with encoding/json conversion;
// see convertStepValue.
func NewTypedStep[ExecIn, ExecOut, CompIn, CompOut any](
	name string,
	execute StepFuncOf[ExecIn, ExecOut],
	compensate StepFuncOf[CompIn, CompOut],
	pivot bool,
) TypedStep {
	return TypedStep{
		name:       name,
		execute:    wrapStepFunc(execute),
		compensate: wrapStepFunc(compensate),
		pivot:      pivot,
	}
}

// NewTypedStepWithoutCompensate returns the TypedStep for the named step
// that needs no rollback: compensate stays nil so the engine skips it.
// It exists because a nil compensating function carries no type arguments
// for NewTypedStep to infer.
func NewTypedStepWithoutCompensate[ExecIn, ExecOut any](
	name string,
	execute StepFuncOf[ExecIn, ExecOut],
	pivot bool,
) TypedStep {
	return TypedStep{
		name:    name,
		execute: wrapStepFunc(execute),
		pivot:   pivot,
	}
}

// AsSagaStep converts s to the engine's untyped SagaStep, preserving name,
// pivot, and nil compensation.
func (s TypedStep) AsSagaStep() SagaStep {
	return SagaStep{
		Name:       s.name,
		Execute:    s.execute,
		Compensate: s.compensate,
		Pivot:      s.pivot,
	}
}

// TypedStepsToSagaSteps converts steps to the engine's untyped shape,
// preserving order. Engines implement TypedSagaRegistrar by converting with
// this helper and delegating to their existing RegisterSaga.
func TypedStepsToSagaSteps(steps []TypedStep) []SagaStep {
	out := make([]SagaStep, 0, len(steps))

	for _, s := range steps {
		out = append(out, s.AsSagaStep())
	}

	return out
}

// TypedSagaRegistrar is implemented by workflow engines that allow hosts to
// register typed saga definitions at runtime. It is a separate interface,
// not part of Workflow and not an extension of SagaRegistrar, so engines
// without saga support are unaffected and no existing implementer breaks.
type TypedSagaRegistrar interface {
	// RegisterTypedSaga registers steps under name, replacing any prior saga.
	RegisterTypedSaga(name string, steps []TypedStep)
}

// wrapStepFunc bridges a typed step function to the engine's untyped
// StepFunc. A nil fn bridges to nil so the engine keeps its skip-nil
// compensation semantics. A nil input decodes to In's zero value; any other
// input converts with encoding/json and failures report ErrTypedStepConvert.
func wrapStepFunc[In, Out any](fn StepFuncOf[In, Out]) StepFunc {
	if fn == nil {
		return nil
	}

	return func(ctx context.Context, input any) (any, error) {
		var in In

		if err := convertStepValue(input, &in); err != nil {
			return nil, err
		}

		out, err := fn(ctx, in)
		if err != nil {
			return nil, err
		}

		return out, nil
	}
}

// convertStepValue decodes input into out with an encoding/json round-trip.
// A nil input leaves out at its zero value.
func convertStepValue(input any, out any) error {
	if input == nil {
		return nil
	}

	raw, err := json.Marshal(input)
	if err != nil {
		return errors.Join(ErrTypedStepConvert, fmt.Errorf("workflow: marshal saga step input: %w", err))
	}

	if err := json.Unmarshal(raw, out); err != nil {
		return errors.Join(ErrTypedStepConvert, fmt.Errorf("workflow: unmarshal saga step input: %w", err))
	}

	return nil
}
