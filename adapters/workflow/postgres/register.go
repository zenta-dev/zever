package postgres

import (
	"github.com/zenta-dev/zever/core/workflow"
)

// Adapter is the postgres workflow adapter name.
const Adapter workflow.Adapter = "postgres"

// Register reserves the adapter name in the workflow registry. Core
// workflow.Options carries no DSN, so the factory fails closed with
// ErrNotConfigured: use New or Open with Options.
func Register() {
	_ = workflow.Register(Adapter, func(workflow.Options) (workflow.Workflow, error) {
		return nil, ErrNotConfigured
	})
}
