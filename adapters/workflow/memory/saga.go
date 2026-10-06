package memory

import (
	"context"
	"fmt"
	"time"

	"github.com/zenta-dev/zever/core/workflow"
	"github.com/zenta-dev/zever/shared/retry"
)

// sagaRetryPolicy bounds roll-forward and compensation retries. Delays are
// small: the memory adapter is in-process, so backoff exists to absorb
// transient failures, not to wait out network partitions.
var sagaRetryPolicy = retry.Policy{
	BaseDelay:   time.Millisecond,
	MaxDelay:    10 * time.Millisecond,
	Multiplier:  2,
	MaxAttempts: 3,
}

// sagaRun is the in-memory state of one saga execution.
type sagaRun struct {
	id          workflow.RunID
	name        string
	workflowID  string
	steps       []workflow.SagaStep
	currentStep int
	status      string
	failedStep  int
	err         string
	executed    []bool
	compensated []bool
	results     []any
}

// Saga memory state: registered saga definitions, runs by ID, and the
// workflowID index that makes RunSaga idempotent. All guarded by Adapter.mu.
// Memory is non-durable: saga state lives only for the Adapter's lifetime;
// Close wipes it. The executed/compensated guards below prevent a step from
// re-running when the same workflowID resumes a non-terminal run.
func (a *Adapter) initSagaStateLocked() {
	if a.sagas == nil {
		a.sagas = make(map[string][]workflow.SagaStep)
	}
	if a.sagaRuns == nil {
		a.sagaRuns = make(map[workflow.RunID]*sagaRun)
	}
	if a.sagaByWorkflow == nil {
		a.sagaByWorkflow = make(map[string]workflow.RunID)
	}
}

// RegisterSaga registers a saga definition under name.
func (a *Adapter) RegisterSaga(name string, steps []workflow.SagaStep) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.initSagaStateLocked()

	a.sagas[name] = steps
}

// RunSaga starts or resumes the named saga. An unknown name fails with
// ErrUnknownSaga. A workflowID matching an existing run returns that run's
// ID without re-executing completed steps (idempotent resume).
func (a *Adapter) RunSaga(ctx context.Context, name string, input any, workflowID string) (workflow.RunID, error) {
	a.mu.Lock()
	a.initSagaStateLocked()

	steps, ok := a.sagas[name]
	if !ok {
		a.mu.Unlock()

		return "", workflow.ErrUnknownSaga
	}

	if workflowID != "" {
		if id, exists := a.sagaByWorkflow[workflowID]; exists {
			if r := a.sagaRuns[id]; r != nil {
				a.mu.Unlock()

				return id, nil
			}
		}
	}

	var id workflow.RunID
	if workflowID != "" {
		id = workflow.RunID(workflowID)
	} else {
		for {
			id = workflow.RunID(fmt.Sprintf("saga-%d", a.nextID))
			if _, exists := a.sagaRuns[id]; !exists {
				break
			}

			a.nextID++
		}

		a.nextID++
	}

	r := &sagaRun{
		id:          id,
		name:        name,
		workflowID:  workflowID,
		steps:       steps,
		status:      workflow.SagaRunning,
		failedStep:  -1,
		executed:    make([]bool, len(steps)),
		compensated: make([]bool, len(steps)),
		results:     make([]any, len(steps)),
	}
	a.sagaRuns[id] = r
	if workflowID != "" {
		a.sagaByWorkflow[workflowID] = id
	}
	a.mu.Unlock()

	a.runSaga(ctx, r, input)

	return id, nil
}

// SagaStatus returns the current status of a saga run.
func (a *Adapter) SagaStatus(_ context.Context, runID workflow.RunID) (workflow.SagaStatus, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	r, ok := a.sagaRuns[runID]
	if !ok {
		return workflow.SagaStatus{}, workflow.ErrSagaNotFound
	}

	return workflow.SagaStatus{
		RunID:       r.id,
		Name:        r.name,
		CurrentStep: r.currentStep,
		Status:      r.status,
		FailedStep:  r.failedStep,
		Err:         r.err,
	}, nil
}

// runSaga executes r forward, compensating in reverse on failure unless a
// pivot step was executed, in which case it rolls forward with retries.
// r.executed guards against re-running steps on resume.
func (a *Adapter) runSaga(ctx context.Context, r *sagaRun, input any) {
	for i := r.currentStep; i < len(r.steps); i++ {
		if r.executed[i] {
			continue
		}

		result, err := r.steps[i].Execute(ctx, input)
		if err != nil {
			a.failSaga(ctx, r, i, err, input)

			return
		}

		a.mu.Lock()
		r.executed[i] = true
		r.results[i] = result
		r.currentStep = i + 1
		a.mu.Unlock()
	}

	a.mu.Lock()
	r.status = workflow.SagaCompleted
	a.mu.Unlock()
}

// failSaga applies the pivot rule: if any executed step before the failure
// has Pivot=true, retry the remaining steps forward with backoff and never
// compensate; otherwise compensate executed steps in reverse order.
func (a *Adapter) failSaga(ctx context.Context, r *sagaRun, failed int, err error, input any) {
	pivot := false

	a.mu.Lock()
	for j := 0; j < failed; j++ {
		if r.executed[j] && r.steps[j].Pivot {
			pivot = true

			break
		}
	}
	a.mu.Unlock()

	if pivot {
		a.rollForward(ctx, r, failed, input)

		return
	}

	a.compensate(ctx, r, failed, err)
}

// rollForward retries the remaining steps from failed onward with backoff.
// Compensation is skipped: the saga is committed to completing forward.
func (a *Adapter) rollForward(ctx context.Context, r *sagaRun, from int, input any) {
	for i := from; i < len(r.steps); i++ {
		if r.executed[i] {
			continue
		}

		var result any

		if rerr := retry.Do(ctx, sagaRetryPolicy, func(ctx context.Context) error {
			res, e := r.steps[i].Execute(ctx, input)
			if e != nil {
				return e
			}

			result = res

			return nil
		}); rerr != nil {
			a.mu.Lock()
			r.status = workflow.SagaFailed
			r.failedStep = i
			r.err = fmt.Errorf("%w: %s", workflow.ErrSagaStepFailed, fmt.Sprintf("step %q: %v", r.steps[i].Name, rerr)).Error()
			a.mu.Unlock()

			return
		}

		a.mu.Lock()
		r.executed[i] = true
		r.results[i] = result
		r.currentStep = i + 1
		a.mu.Unlock()
	}

	a.mu.Lock()
	r.status = workflow.SagaCompleted
	a.mu.Unlock()
}

// compensate rolls back executed steps before failed in reverse order,
// skipping nil Compensate. Any compensation failure marks the run
// SagaCompensationFailed; otherwise it is SagaFailed.
func (a *Adapter) compensate(ctx context.Context, r *sagaRun, failed int, cause error) {
	a.mu.Lock()
	r.status = workflow.SagaCompensating
	r.failedStep = failed
	r.err = fmt.Errorf("%w: %s", workflow.ErrSagaStepFailed, fmt.Sprintf("step %q: %v", r.steps[failed].Name, cause)).Error()
	a.mu.Unlock()

	compFailed := false

	for j := failed - 1; j >= 0; j-- {
		a.mu.RLock()
		executed := r.executed[j]
		comp := r.steps[j].Compensate
		result := r.results[j]
		a.mu.RUnlock()

		if !executed || comp == nil {
			continue
		}

		if err := retry.Do(ctx, sagaRetryPolicy, func(ctx context.Context) error {
			_, e := comp(ctx, result)

			return e
		}); err != nil {
			compFailed = true

			a.mu.Lock()
			r.err = fmt.Errorf("%w: %s", workflow.ErrSagaCompensationFailed, fmt.Sprintf("step %q: %v", r.steps[j].Name, err)).Error()
			a.mu.Unlock()
		}

		a.mu.Lock()
		r.compensated[j] = true
		a.mu.Unlock()
	}

	a.mu.Lock()
	if compFailed {
		r.status = workflow.SagaCompensationFailed
	} else {
		r.status = workflow.SagaFailed
	}
	a.mu.Unlock()
}

var (
	_ workflow.SagaRegistrar = (*Adapter)(nil)
	_ workflow.SagaRunner    = (*Adapter)(nil)
	_ workflow.SagaInspector = (*Adapter)(nil)
)
