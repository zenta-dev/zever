package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/zenta-dev/zever/core/workflow"
	"github.com/zenta-dev/zever/orm"
	"github.com/zenta-dev/zever/shared/retry"
)

// Saga table names created when missing.
const (
	// DefaultSagaTable holds one row per saga run.
	DefaultSagaTable = "workflow_saga_runs"
	// DefaultSagaCompensationsTable holds one row per compensation attempt.
	DefaultSagaCompensationsTable = "workflow_saga_compensations"
)

// Saga run statuses persisted in the status column.
const (
	sagaStateRunning            = workflow.SagaRunning
	sagaStateCompleted          = workflow.SagaCompleted
	sagaStateCompensating       = workflow.SagaCompensating
	sagaStateFailed             = workflow.SagaFailed
	sagaStateCompensationFailed = workflow.SagaCompensationFailed
)

// sagaRetryPolicy bounds roll-forward and compensation retries.
var sagaRetryPolicy = retry.Policy{
	BaseDelay:   5 * time.Millisecond,
	MaxDelay:    50 * time.Millisecond,
	Multiplier:  2,
	MaxAttempts: 3,
}

// sagaState is the JSON document persisted in the runs table state column:
// the original input plus one entry per completed step result.
type sagaState struct {
	Input   any   `json:"input"`
	Results []any `json:"results"`
}

// sagaRunRow is the workflow_saga_runs entity. Column order matches
// sagaRunColumns: the positional Scan must read them in exactly this order.
type sagaRunRow struct {
	RunID       string
	Name        string
	WorkflowID  string
	Status      string
	CurrentStep int
	State       string
	FailedStep  int
	Err         string
	LockedUntil time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// sagaRunColumns is the entity column list in Scan order.
var sagaRunColumns = []string{
	"run_id", "name", "workflow_id", "status", "current_step", "state",
	"failed_step", "err", "locked_until", "created_at", "updated_at",
}

// Scan reads one row positionally, coercing driver representations.
func (r *sagaRunRow) Scan(row orm.Row) error {
	var id, name, wfID, status, state, errText string
	var currentStep, failedStep int
	var lockedRaw, createdRaw, updatedRaw any

	if err := row.Scan(&id, &name, &wfID, &status, &currentStep, &state,
		&failedStep, &errText, &lockedRaw, &createdRaw, &updatedRaw); err != nil {
		return err
	}

	locked, err := coerceTime(lockedRaw)
	if err != nil {
		return fmt.Errorf("postgres: scan locked_until: %w", err)
	}

	created, err := coerceTime(createdRaw)
	if err != nil {
		return fmt.Errorf("postgres: scan created_at: %w", err)
	}

	updated, err := coerceTime(updatedRaw)
	if err != nil {
		return fmt.Errorf("postgres: scan updated_at: %w", err)
	}

	*r = sagaRunRow{
		RunID: id, Name: name, WorkflowID: wfID, Status: status,
		CurrentStep: currentStep, State: state, FailedStep: failedStep,
		Err: errText, LockedUntil: locked, CreatedAt: created, UpdatedAt: updated,
	}

	return nil
}

// sagaCompensationRow is the workflow_saga_compensations entity.
type sagaCompensationRow struct {
	RunID     string
	StepIndex int
	Name      string
	Attempts  int
	LastError string
}

// sagaCompensationColumns is the entity column list in Scan order.
var sagaCompensationColumns = []string{"run_id", "step_index", "name", "attempts", "last_error"}

// Scan reads one row positionally.
func (r *sagaCompensationRow) Scan(row orm.Row) error {
	var id, name, lastErr string
	var stepIndex, attempts int

	if err := row.Scan(&id, &stepIndex, &name, &attempts, &lastErr); err != nil {
		return err
	}

	*r = sagaCompensationRow{
		RunID: id, StepIndex: stepIndex, Name: name, Attempts: attempts, LastError: lastErr,
	}

	return nil
}

// sagaTables wires the saga ORM handles onto d. Called from openFromDB.
func (d *driver) sagaTables() {
	d.sagaTbl = orm.NewTable[sagaRunRow](DefaultSagaTable, sagaRunColumns)
	d.sagaRunID = orm.NewColumn[sagaRunRow, string](DefaultSagaTable, "run_id")
	d.sagaName = orm.NewColumn[sagaRunRow, string](DefaultSagaTable, "name")
	d.sagaWorkflowID = orm.NewColumn[sagaRunRow, string](DefaultSagaTable, "workflow_id")
	d.sagaStatus = orm.NewColumn[sagaRunRow, string](DefaultSagaTable, "status")
	d.sagaCurrentStep = orm.NewColumn[sagaRunRow, int](DefaultSagaTable, "current_step")
	d.sagaStateCol = orm.NewColumn[sagaRunRow, string](DefaultSagaTable, "state")
	d.sagaFailedStep = orm.NewColumn[sagaRunRow, int](DefaultSagaTable, "failed_step")
	d.sagaErr = orm.NewColumn[sagaRunRow, string](DefaultSagaTable, "err")
	d.sagaLockedUntil = orm.NewColumn[sagaRunRow, time.Time](DefaultSagaTable, "locked_until")
	d.sagaCreatedAt = orm.NewColumn[sagaRunRow, time.Time](DefaultSagaTable, "created_at")
	d.sagaUpdatedAt = orm.NewColumn[sagaRunRow, time.Time](DefaultSagaTable, "updated_at")

	d.sagaCompTbl = orm.NewTable[sagaCompensationRow](DefaultSagaCompensationsTable, sagaCompensationColumns)
	d.sagaCompRunID = orm.NewColumn[sagaCompensationRow, string](DefaultSagaCompensationsTable, "run_id")
	d.sagaCompStepIndex = orm.NewColumn[sagaCompensationRow, int](DefaultSagaCompensationsTable, "step_index")
	d.sagaCompName = orm.NewColumn[sagaCompensationRow, string](DefaultSagaCompensationsTable, "name")
	d.sagaCompAttempts = orm.NewColumn[sagaCompensationRow, int](DefaultSagaCompensationsTable, "attempts")
	d.sagaCompLastError = orm.NewColumn[sagaCompensationRow, string](DefaultSagaCompensationsTable, "last_error")
}

// ensureSagaSchema creates the saga tables when missing. DDL only: every
// state transition below goes through the orm typed builder.
func (d *driver) ensureSagaSchema(ctx context.Context) error {
	sagaQuoted := `"` + strings.ReplaceAll(DefaultSagaTable, `"`, `""`) + `"`
	compQuoted := `"` + strings.ReplaceAll(DefaultSagaCompensationsTable, `"`, `""`) + `"`

	var runsDDL, compsDDL string

	if d.conn.Dialect() == "postgres" {
		runsDDL = `CREATE TABLE IF NOT EXISTS ` + sagaQuoted + ` (` +
			`run_id TEXT PRIMARY KEY, ` +
			`name TEXT NOT NULL, ` +
			`workflow_id TEXT NOT NULL, ` +
			`status TEXT NOT NULL, ` +
			`current_step INTEGER NOT NULL DEFAULT 0, ` +
			`state TEXT NOT NULL DEFAULT '', ` +
			`failed_step INTEGER NOT NULL DEFAULT -1, ` +
			`err TEXT NOT NULL DEFAULT '', ` +
			`locked_until TIMESTAMPTZ NOT NULL, ` +
			`created_at TIMESTAMPTZ NOT NULL, ` +
			`updated_at TIMESTAMPTZ NOT NULL)`
		compsDDL = `CREATE TABLE IF NOT EXISTS ` + compQuoted + ` (` +
			`run_id TEXT NOT NULL, ` +
			`step_index INTEGER NOT NULL, ` +
			`name TEXT NOT NULL, ` +
			`attempts INTEGER NOT NULL DEFAULT 0, ` +
			`last_error TEXT NOT NULL DEFAULT '', ` +
			`PRIMARY KEY (run_id, step_index))`
	} else {
		runsDDL = `CREATE TABLE IF NOT EXISTS ` + sagaQuoted + ` (` +
			`run_id TEXT PRIMARY KEY, ` +
			`name TEXT NOT NULL, ` +
			`workflow_id TEXT NOT NULL, ` +
			`status TEXT NOT NULL, ` +
			`current_step INTEGER NOT NULL DEFAULT 0, ` +
			`state TEXT NOT NULL DEFAULT '', ` +
			`failed_step INTEGER NOT NULL DEFAULT -1, ` +
			`err TEXT NOT NULL DEFAULT '', ` +
			`locked_until TEXT NOT NULL, ` +
			`created_at TEXT NOT NULL, ` +
			`updated_at TEXT NOT NULL)`
		compsDDL = `CREATE TABLE IF NOT EXISTS ` + compQuoted + ` (` +
			`run_id TEXT NOT NULL, ` +
			`step_index INTEGER NOT NULL, ` +
			`name TEXT NOT NULL, ` +
			`attempts INTEGER NOT NULL DEFAULT 0, ` +
			`last_error TEXT NOT NULL DEFAULT '', ` +
			`PRIMARY KEY (run_id, step_index))`
	}

	if _, err := d.conn.Exec(ctx, runsDDL); err != nil {
		return fmt.Errorf("postgres: ensure saga schema: %w", err)
	}

	if _, err := d.conn.Exec(ctx, compsDDL); err != nil {
		return fmt.Errorf("postgres: ensure saga schema: %w", err)
	}

	return nil
}

// RegisterSaga registers a saga definition under name. Definitions are
// in-memory (steps are funcs); only run progress is durable. A recovered
// run whose definition is missing cannot resume and is skipped by
// RecoverStuckSagas.
func (d *driver) RegisterSaga(name string, steps []workflow.SagaStep) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.sagas == nil {
		d.sagas = make(map[string][]workflow.SagaStep)
	}

	d.sagas[name] = steps
}

// sagaSteps returns the saga definition registered under name.
func (d *driver) sagaSteps(name string) ([]workflow.SagaStep, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	steps, ok := d.sagas[name]

	return steps, ok
}

// RunSaga starts or resumes the named saga. An unknown name fails with
// ErrUnknownSaga. A workflowID matching an existing run resumes that run
// instead of restarting it: terminal runs return their ID unchanged,
// non-terminal runs continue from the persisted current_step (the
// run_id:step_index guard — a step is never re-executed after its
// progress is persisted).
func (d *driver) RunSaga(ctx context.Context, name string, input any, workflowID string) (workflow.RunID, error) {
	if err := d.checkDialect(); err != nil {
		return "", err
	}

	steps, ok := d.sagaSteps(name)
	if !ok {
		return "", workflow.ErrUnknownSaga
	}

	if workflowID != "" {
		row, err := d.loadSagaByWorkflowID(ctx, workflowID)
		if err != nil && !errors.Is(err, workflow.ErrSagaNotFound) {
			return "", err
		}

		if err == nil {
			id := workflow.RunID(row.RunID)

			if !sagaStatusTerminal(row.Status) {
				state, derr := decodeSagaState(row.State)
				if derr != nil {
					return "", derr
				}

				d.runSaga(ctx, id, name, steps, state.Input, row.CurrentStep, row.Status)
			}

			return id, nil
		}
	}

	inputJSON, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("postgres: run saga %q: marshal input: %w", name, err)
	}

	now := time.Now().UTC()
	stateJSON := fmt.Sprintf(`{"input":%s,"results":[]}`, inputJSON)

	var id workflow.RunID
	if workflowID != "" {
		id = workflow.RunID(workflowID)
	} else {
		id = workflow.RunID(fmt.Sprintf("saga-%d", d.nextID.Add(1)))
	}

	err = orm.InsertInto(d.sagaTbl).Values(
		orm.Set(d.sagaRunID, string(id)),
		orm.Set(d.sagaName, name),
		orm.Set(d.sagaWorkflowID, workflowID),
		orm.Set(d.sagaStatus, sagaStateRunning),
		orm.Set(d.sagaCurrentStep, 0),
		orm.Set(d.sagaStateCol, stateJSON),
		orm.Set(d.sagaFailedStep, -1),
		orm.Set(d.sagaLockedUntil, now.Add(d.leaseTTL)),
		orm.Set(d.sagaCreatedAt, now),
		orm.Set(d.sagaUpdatedAt, now),
	).Exec(ctx, d.conn)
	if err != nil {
		if isDuplicateErr(err) && workflowID != "" {
			return "", workflow.DuplicateRunError{RunID: workflowID}
		}

		return "", err
	}

	d.runSaga(ctx, id, name, steps, input, 0, sagaStateRunning)

	return id, nil
}

// SagaStatus returns the current status of a saga run.
func (d *driver) SagaStatus(ctx context.Context, runID workflow.RunID) (workflow.SagaStatus, error) {
	if err := d.checkDialect(); err != nil {
		return workflow.SagaStatus{}, err
	}

	row, err := d.loadSaga(ctx, runID)
	if err != nil {
		return workflow.SagaStatus{}, err
	}

	return workflow.SagaStatus{
		RunID:       workflow.RunID(row.RunID),
		Name:        row.Name,
		CurrentStep: row.CurrentStep,
		Status:      row.Status,
		FailedStep:  row.FailedStep,
		Err:         row.Err,
	}, nil
}

// RecoverStuckSagas resumes saga runs stuck in running or compensating whose
// lease expired at least olderThan ago. Each recovered run continues from
// its persisted current_step; runs whose saga definition is no longer
// registered are skipped. Returns the number of runs resumed. Explicit
// caller-driven recovery: no background goroutine.
func (d *driver) RecoverStuckSagas(ctx context.Context, olderThan time.Duration) (int, error) {
	if err := d.checkDialect(); err != nil {
		return 0, err
	}

	threshold := time.Now().UTC().Add(-olderThan)

	rows, err := orm.From[sagaRunRow, *sagaRunRow](d.sagaTbl).Where(orm.And(
		d.sagaStatus.In(sagaStateRunning, sagaStateCompensating),
		d.sagaLockedUntil.Lte(threshold),
	)).All(ctx, d.conn)
	if err != nil {
		return 0, err
	}

	recovered := 0

	for _, row := range rows {
		steps, ok := d.sagaSteps(row.Name)
		if !ok {
			continue
		}

		state, derr := decodeSagaState(row.State)
		if derr != nil {
			return recovered, derr
		}

		d.runSaga(ctx, workflow.RunID(row.RunID), row.Name, steps, state.Input, row.CurrentStep, row.Status)
		recovered++
	}

	return recovered, nil
}

// runSaga executes the saga from startStep, persisting progress after each
// successful step. On failure it rolls forward (pivot) or compensates in
// reverse, persisting compensation failures. resumeStatus is the persisted
// status on entry: running executes forward, compensating continues the
// reverse rollback.
func (d *driver) runSaga(ctx context.Context, id workflow.RunID, name string, steps []workflow.SagaStep, input any, startStep int, resumeStatus string) {
	if resumeStatus == sagaStateCompensating {
		d.compensateSaga(ctx, id, steps, startStep-1)

		return
	}

	failed := -1
	var stepErr error

	for i := startStep; i < len(steps); i++ {
		result, err := steps[i].Execute(ctx, input)
		if err != nil {
			failed = i
			stepErr = err

			break
		}

		if perr := d.persistSagaProgress(ctx, id, i+1, result); perr != nil {
			_ = d.updateSagaStatus(ctx, id, sagaStateFailed, i, i, perr.Error())

			return
		}
	}

	if failed == -1 {
		_ = d.updateSagaStatus(ctx, id, sagaStateCompleted, len(steps), -1, "")

		return
	}

	if d.sagaHasPivot(steps, failed) {
		d.rollSagaForward(ctx, id, steps, failed, input)

		return
	}

	_ = d.updateSagaStatus(ctx, id, sagaStateCompensating, failed, failed,
		fmt.Errorf("%w: step %q: %w", workflow.ErrSagaStepFailed, steps[failed].Name, stepErr).Error())
	d.compensateSaga(ctx, id, steps, failed-1)
}

// sagaHasPivot reports whether any executed step before failed has
// Pivot=true.
func (d *driver) sagaHasPivot(steps []workflow.SagaStep, failed int) bool {
	for j := 0; j < failed; j++ {
		if steps[j].Pivot {
			return true
		}
	}

	return false
}

// rollSagaForward retries the remaining steps from failed onward with
// backoff. Compensation is skipped: the saga is committed to completing
// forward. Exhausting retries marks the run SagaFailed.
func (d *driver) rollSagaForward(ctx context.Context, id workflow.RunID, steps []workflow.SagaStep, from int, input any) {
	for i := from; i < len(steps); i++ {
		var result any

		if rerr := retry.Do(ctx, sagaRetryPolicy, func(ctx context.Context) error {
			res, e := steps[i].Execute(ctx, input)
			if e != nil {
				return e
			}

			result = res

			return nil
		}); rerr != nil {
			_ = d.updateSagaStatus(ctx, id, sagaStateFailed, i, i,
				fmt.Errorf("%w: step %q: %w", workflow.ErrSagaStepFailed, steps[i].Name, rerr).Error())

			return
		}

		if perr := d.persistSagaProgress(ctx, id, i+1, result); perr != nil {
			_ = d.updateSagaStatus(ctx, id, sagaStateFailed, i, i, perr.Error())

			return
		}
	}

	_ = d.updateSagaStatus(ctx, id, sagaStateCompleted, len(steps), -1, "")
}

// compensateSaga rolls back executed steps from from down to 0 in reverse
// order, skipping nil Compensate and steps already compensated before a
// crash. Attempts and failures are recorded in
// workflow_saga_compensations. Any failure marks the run
// SagaCompensationFailed; otherwise it is SagaFailed.
func (d *driver) compensateSaga(ctx context.Context, id workflow.RunID, steps []workflow.SagaStep, from int) {
	state, err := d.loadSagaState(ctx, id)
	if err != nil {
		_ = d.updateSagaStatus(ctx, id, sagaStateCompensationFailed, from+1, from+1, err.Error())

		return
	}

	done := d.loadSuccessfulCompensations(ctx, id)

	compFailed := false

	for j := from; j >= 0; j-- {
		if steps[j].Compensate == nil || done[j] {
			continue
		}

		var input any
		if j < len(state.Results) {
			input = state.Results[j]
		}

		compErr := d.runCompensation(ctx, id, j, steps[j], input)
		if compErr != nil {
			compFailed = true
		}
	}

	if compFailed {
		_ = d.updateSagaStatus(ctx, id, sagaStateCompensationFailed, from+1, from+1,
			fmt.Errorf("%w: saga %q", workflow.ErrSagaCompensationFailed, id).Error())

		return
	}

	_ = d.updateSagaStatus(ctx, id, sagaStateFailed, from+1, from+1, d.loadErr(ctx, id))
}

// runCompensation runs one compensation with bounded retry, recording the
// attempt count and last error. Returns the final error, nil on success.
func (d *driver) runCompensation(ctx context.Context, id workflow.RunID, stepIndex int, step workflow.SagaStep, input any) error {
	attempts := 0

	lastErr := retry.Do(ctx, sagaRetryPolicy, func(ctx context.Context) error {
		attempts++

		_, err := step.Compensate(ctx, input)

		return err
	})

	lastErrStr := ""
	if lastErr != nil {
		lastErrStr = lastErr.Error()
	}

	d.recordCompensation(ctx, id, stepIndex, step.Name, attempts, lastErrStr)

	return lastErr
}

// persistSagaProgress records step index i+1 as done with its result,
// extending the lease so a live run is not recovered mid-flight.
func (d *driver) persistSagaProgress(ctx context.Context, id workflow.RunID, step int, result any) error {
	state, err := d.loadSagaState(ctx, id)
	if err != nil {
		return err
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("postgres: saga %q: marshal result: %w", id, err)
	}

	state.Results = append(state.Results, json.RawMessage(resultJSON))

	stateJSON, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("postgres: saga %q: marshal state: %w", id, err)
	}

	now := time.Now().UTC()

	n, err := orm.UpdateTable(d.sagaTbl).Where(d.sagaRunID.Eq(string(id))).Set(
		orm.Set(d.sagaCurrentStep, step),
		orm.Set(d.sagaStateCol, string(stateJSON)),
		orm.Set(d.sagaLockedUntil, now.Add(d.leaseTTL)),
		orm.Set(d.sagaUpdatedAt, now),
	).Exec(ctx, d.conn)
	if err != nil {
		return err
	}

	if n == 0 {
		return fmt.Errorf("postgres: saga %q: run vanished", id)
	}

	return nil
}

// updateSagaStatus writes the terminal or in-progress status, failed step,
// and error message.
func (d *driver) updateSagaStatus(ctx context.Context, id workflow.RunID, status string, currentStep, failedStep int, errMsg string) error {
	now := time.Now().UTC()

	_, err := orm.UpdateTable(d.sagaTbl).Where(d.sagaRunID.Eq(string(id))).Set(
		orm.Set(d.sagaStatus, status),
		orm.Set(d.sagaCurrentStep, currentStep),
		orm.Set(d.sagaFailedStep, failedStep),
		orm.Set(d.sagaErr, errMsg),
		orm.Set(d.sagaUpdatedAt, now),
	).Exec(ctx, d.conn)

	return err
}

// recordCompensation upserts one compensation attempt row.
func (d *driver) recordCompensation(ctx context.Context, id workflow.RunID, stepIndex int, name string, attempts int, lastErr string) {
	_ = orm.InsertInto(d.sagaCompTbl).Values(
		orm.Set(d.sagaCompRunID, string(id)),
		orm.Set(d.sagaCompStepIndex, stepIndex),
		orm.Set(d.sagaCompName, name),
		orm.Set(d.sagaCompAttempts, attempts),
		orm.Set(d.sagaCompLastError, lastErr),
	).Exec(ctx, d.conn)
}

// loadSuccessfulCompensations returns the set of step indexes whose
// compensation already succeeded (no recorded error) before a crash.
func (d *driver) loadSuccessfulCompensations(ctx context.Context, id workflow.RunID) map[int]bool {
	rows, err := orm.From[sagaCompensationRow, *sagaCompensationRow](d.sagaCompTbl).
		Where(d.sagaCompRunID.Eq(string(id))).All(ctx, d.conn)
	if err != nil {
		return nil
	}

	done := make(map[int]bool, len(rows))
	for _, r := range rows {
		if r.LastError == "" {
			done[r.StepIndex] = true
		}
	}

	return done
}

// loadErr returns the persisted error message for id, empty on failure.
func (d *driver) loadErr(ctx context.Context, id workflow.RunID) string {
	row, err := d.loadSaga(ctx, id)
	if err != nil {
		return ""
	}

	return row.Err
}

// loadSaga fetches one saga run row by id.
func (d *driver) loadSaga(ctx context.Context, id workflow.RunID) (*sagaRunRow, error) {
	row, ok, err := orm.From[sagaRunRow, *sagaRunRow](d.sagaTbl).Where(d.sagaRunID.Eq(string(id))).First(ctx, d.conn)
	if err != nil {
		return nil, err
	}

	if !ok {
		return nil, workflow.ErrSagaNotFound
	}

	return row, nil
}

// loadSagaByWorkflowID fetches one saga run row by its workflow ID.
func (d *driver) loadSagaByWorkflowID(ctx context.Context, workflowID string) (*sagaRunRow, error) {
	row, ok, err := orm.From[sagaRunRow, *sagaRunRow](d.sagaTbl).Where(d.sagaWorkflowID.Eq(workflowID)).First(ctx, d.conn)
	if err != nil {
		return nil, err
	}

	if !ok {
		return nil, workflow.ErrSagaNotFound
	}

	return row, nil
}

// loadSagaState reads and decodes the state document for id.
func (d *driver) loadSagaState(ctx context.Context, id workflow.RunID) (*sagaState, error) {
	row, err := d.loadSaga(ctx, id)
	if err != nil {
		return nil, err
	}

	return decodeSagaState(row.State)
}

// decodeSagaState parses a state document, tolerating an empty string.
func decodeSagaState(raw string) (*sagaState, error) {
	state := &sagaState{}
	if raw == "" {
		return state, nil
	}

	if err := json.Unmarshal([]byte(raw), state); err != nil {
		return nil, fmt.Errorf("postgres: decode saga state: %w", err)
	}

	return state, nil
}

// sagaStatusTerminal reports whether status ends the saga lifecycle.
func sagaStatusTerminal(status string) bool {
	return status == sagaStateCompleted || status == sagaStateFailed || status == sagaStateCompensationFailed
}

var (
	_ workflow.SagaRegistrar = (*driver)(nil)
	_ workflow.SagaRunner    = (*driver)(nil)
	_ workflow.SagaInspector = (*driver)(nil)
)
