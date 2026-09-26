package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/zenta-dev/zever/core/workflow"
)

type run struct {
	name    string
	input   any
	state   any
	running bool
	pending []any
}

// Adapter is an in-memory workflow engine for testing and development.
type Adapter struct {
	mu     sync.RWMutex
	runs   map[workflow.RunID]*run
	steps  map[string]workflow.StepFunc
	nextID uint64
}

// New creates an in-memory workflow engine.
func New(o workflow.Options) (workflow.Workflow, error) {
	if err := o.Validate(); err != nil {
		return nil, fmt.Errorf("memory: %w", err)
	}

	return &Adapter{
		runs:   make(map[workflow.RunID]*run),
		steps:  make(map[string]workflow.StepFunc),
		nextID: 1,
	}, nil
}

// RegisterStep registers a named step function.
func (a *Adapter) RegisterStep(name string, fn workflow.StepFunc) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.steps == nil {
		a.steps = make(map[string]workflow.StepFunc)
	}

	a.steps[name] = fn
}

// Start begins a new workflow run.
func (a *Adapter) Start(ctx context.Context, name string, input any, workflowID string) (workflow.RunID, error) {
	a.mu.Lock()
	if a.runs == nil {
		a.runs = make(map[workflow.RunID]*run)
	}

	if a.steps == nil {
		a.steps = make(map[string]workflow.StepFunc)
	}

	var id workflow.RunID
	if workflowID != "" {
		id = workflow.RunID(workflowID)
		if _, exists := a.runs[id]; exists {
			a.mu.Unlock()

			return "", &workflow.DuplicateRunError{RunID: workflowID}
		}
	} else {
		for {
			id = workflow.RunID(fmt.Sprintf("run-%d", a.nextID))
			if _, exists := a.runs[id]; !exists {
				break
			}

			a.nextID++
		}

		a.nextID++
	}

	r := &run{name: name, input: input, state: input, running: true}
	a.runs[id] = r
	fn, ok := a.steps[name]
	a.mu.Unlock()

	if !ok {
		a.mu.Lock()
		if cur, self := a.runs[id]; self && cur == r {
			delete(a.runs, id)
		}
		a.mu.Unlock()

		return "", &workflow.UnknownStepError{Step: name}
	}

	result, err := fn(ctx, input)
	if err != nil {
		a.mu.Lock()
		if cur, self := a.runs[id]; self && cur == r {
			delete(a.runs, id)
		}
		a.mu.Unlock()

		return "", fmt.Errorf("workflow: step %q failed: %w", name, err)
	}

	a.mu.Lock()
	if cur, ok := a.runs[id]; ok && cur == r {
		cur.state = result
		for _, v := range cur.pending {
			cur.state = v
		}

		cur.pending = nil
		cur.running = false
	}
	a.mu.Unlock()

	return id, nil
}

// Signal sends a signal to a running workflow.
func (a *Adapter) Signal(_ context.Context, runID workflow.RunID, _ string, value any) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	r, ok := a.runs[runID]
	if !ok {
		return &workflow.UnknownRunError{RunID: runID}
	}

	if !r.running {
		return &workflow.RunCompletedError{RunID: runID}
	}

	if len(r.pending) >= 100 {
		return &workflow.PendingFullError{RunID: runID}
	}

	r.pending = append(r.pending, value)

	return nil
}

// Query returns the state of a running workflow.
func (a *Adapter) Query(_ context.Context, runID workflow.RunID, name string, out any) error {
	a.mu.RLock()

	r, ok := a.runs[runID]
	if !ok {
		a.mu.RUnlock()
		return &workflow.UnknownRunError{RunID: runID}
	}

	if name != "state" {
		a.mu.RUnlock()
		return &workflow.UnknownQueryError{Query: name}
	}

	state := r.state

	a.mu.RUnlock()

	b, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("workflow: query %q: marshal state: %w", name, err)
	}

	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("workflow: query %q: type mismatch: %w", name, err)
	}

	return nil
}

// Cancel stops a running workflow.
func (a *Adapter) Cancel(_ context.Context, runID workflow.RunID) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	r, ok := a.runs[runID]
	if !ok {
		return &workflow.UnknownRunError{RunID: runID}
	}

	if !r.running {
		return &workflow.RunCompletedError{RunID: runID}
	}

	delete(a.runs, runID)

	return nil
}

// Close releases all resources held by the in-memory workflow engine.
func (a *Adapter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.runs = nil
	a.steps = nil

	return nil
}
