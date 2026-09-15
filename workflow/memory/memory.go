package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/zenta-dev/zever/workflow"
)

type stepFn func(ctx context.Context, input any) (any, error)

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
	steps  map[string]stepFn
	nextID uint64
}

// New creates an in-memory workflow engine.
func New(o workflow.Options) (workflow.Workflow, error) {
	if err := o.Validate(); err != nil {
		return nil, fmt.Errorf("memory: %w", err)
	}

	return &Adapter{
		runs:   make(map[workflow.RunID]*run),
		steps:  make(map[string]stepFn),
		nextID: 1,
	}, nil
}

// RegisterStep registers a named step function.
func (m *Adapter) RegisterStep(name string, fn stepFn) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.steps == nil {
		m.steps = make(map[string]stepFn)
	}

	m.steps[name] = fn
}

// Start begins a new workflow run.
func (m *Adapter) Start(ctx context.Context, name string, input any, workflowID string) (workflow.RunID, error) {
	m.mu.Lock()
	if m.runs == nil {
		m.runs = make(map[workflow.RunID]*run)
	}

	if m.steps == nil {
		m.steps = make(map[string]stepFn)
	}

	var id workflow.RunID
	if workflowID != "" {
		id = workflow.RunID(workflowID)
		if _, exists := m.runs[id]; exists {
			m.mu.Unlock()

			return "", &workflow.DuplicateRunError{RunID: workflowID}
		}
	} else {
		for {
			id = workflow.RunID(fmt.Sprintf("run-%d", m.nextID))
			if _, exists := m.runs[id]; !exists {
				break
			}

			m.nextID++
		}

		m.nextID++
	}

	r := &run{name: name, input: input, state: input, running: true}
	m.runs[id] = r
	fn, ok := m.steps[name]
	m.mu.Unlock()

	if !ok {
		m.mu.Lock()
		if cur, self := m.runs[id]; self && cur == r {
			delete(m.runs, id)
		}
		m.mu.Unlock()

		return "", &workflow.UnknownStepError{Step: name}
	}

	result, err := fn(ctx, input)
	if err != nil {
		m.mu.Lock()
		if cur, self := m.runs[id]; self && cur == r {
			delete(m.runs, id)
		}
		m.mu.Unlock()

		return "", fmt.Errorf("workflow: step %q failed: %w", name, err)
	}

	m.mu.Lock()
	if cur, ok := m.runs[id]; ok && cur == r {
		cur.state = result
		for _, v := range cur.pending {
			cur.state = v
		}

		cur.pending = nil
		cur.running = false
	}
	m.mu.Unlock()

	return id, nil
}

// Signal sends a signal to a running workflow.
func (m *Adapter) Signal(_ context.Context, runID workflow.RunID, _ string, value any) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	r, ok := m.runs[runID]
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
func (m *Adapter) Query(_ context.Context, runID workflow.RunID, name string, out any) error {
	m.mu.RLock()

	r, ok := m.runs[runID]
	if !ok {
		m.mu.RUnlock()
		return &workflow.UnknownRunError{RunID: runID}
	}

	if name != "state" {
		m.mu.RUnlock()
		return &workflow.UnknownQueryError{Query: name}
	}

	state := r.state

	m.mu.RUnlock()

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
func (m *Adapter) Cancel(_ context.Context, runID workflow.RunID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	r, ok := m.runs[runID]
	if !ok {
		return &workflow.UnknownRunError{RunID: runID}
	}

	if !r.running {
		return &workflow.RunCompletedError{RunID: runID}
	}

	delete(m.runs, runID)

	return nil
}

// Close releases all resources held by the in-memory workflow engine.
func (m *Adapter) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.runs = nil
	m.steps = nil

	return nil
}
