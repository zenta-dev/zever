package workflow

import "context"

// Saga status values reported by SagaStatus.Status.
const (
	// SagaPending is a saga run queued but not yet started.
	SagaPending = "pending"
	// SagaRunning is a saga run executing steps forward.
	SagaRunning = "running"
	// SagaCompleted is a saga run whose steps all succeeded.
	SagaCompleted = "completed"
	// SagaCompensating is a saga run rolling back executed steps.
	SagaCompensating = "compensating"
	// SagaFailed is a saga run that failed and was compensated (or
	// exhausted pivot roll-forward retries).
	SagaFailed = "failed"
	// SagaCompensationFailed is a saga run whose compensation itself
	// failed; manual intervention may be required.
	SagaCompensationFailed = "compensation_failed"
)

// SagaStep is one forward action in a saga plus its optional reverse
// compensation. A nil Compensate means the step needs no rollback.
type SagaStep struct {
	// Name identifies the step for status reporting.
	Name string
	// Execute runs the forward action.
	Execute StepFunc
	// Compensate undoes Execute. Nil means no compensation.
	Compensate StepFunc
	// Pivot marks a step after which failures roll forward (retry the
	// remaining steps) instead of compensating. Once any executed step
	// before the failure has Pivot=true, compensation is skipped.
	Pivot bool
}

// SagaRegistrar is implemented by workflow engines that allow hosts to
// register saga definitions at runtime. Like StepRegistrar it is a
// separate interface, not part of Workflow, so engines without saga
// support are unaffected.
type SagaRegistrar interface {
	// RegisterSaga registers steps under name, replacing any prior saga.
	RegisterSaga(name string, steps []SagaStep)
}

// SagaRunner executes registered sagas.
type SagaRunner interface {
	// RunSaga starts (or resumes) the named saga with input, keyed by
	// workflowID for idempotency. An empty workflowID generates one.
	RunSaga(ctx context.Context, name string, input any, workflowID string) (RunID, error)
}

// SagaInspector reports saga run status.
type SagaInspector interface {
	// SagaStatus returns the current status of a saga run.
	SagaStatus(ctx context.Context, runID RunID) (SagaStatus, error)
}

// SagaStatus is a point-in-time view of a saga run.
type SagaStatus struct {
	// RunID identifies the saga run.
	RunID RunID
	// Name is the registered saga name.
	Name string
	// CurrentStep is the index of the next step to execute.
	CurrentStep int
	// Status is one of the Saga* status constants.
	Status string
	// FailedStep is the index of the step that failed, or -1.
	FailedStep int
	// Err holds the failure message, empty when none.
	Err string
}
