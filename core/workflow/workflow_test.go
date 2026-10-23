package workflow_test

import (
	"context"

	"github.com/zenta-dev/zever/core/workflow"
)

type stubWorkflow struct{}

func (stubWorkflow) Start(_ context.Context, _ string, _ any, _ string) (workflow.RunID, error) {
	return "run-1", nil
}

func (stubWorkflow) Signal(_ context.Context, _ workflow.RunID, _ string, _ any) error { return nil }

func (stubWorkflow) Query(_ context.Context, _ workflow.RunID, _ string, _ any) error { return nil }

func (stubWorkflow) Cancel(_ context.Context, _ workflow.RunID) error { return nil }

func (stubWorkflow) Close() error { return nil }

var _ workflow.Workflow = stubWorkflow{}
