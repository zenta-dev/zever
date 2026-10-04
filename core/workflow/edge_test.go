package workflow_test

import (
	"errors"
	"strings"
	"sync"
	"testing"

	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/workflow"
)

func TestOpenShared_success(t *testing.T) {
	t.Parallel()

	a := freshWorkflowAdapter()
	if err := workflow.RegisterShared(a, func(coredb.DB, workflow.Options) (workflow.Workflow, error) {
		return stubWorkflow{}, nil
	}); err != nil {
		t.Fatalf("RegisterShared(%v) error = %v", a, err)
	}

	w, err := workflow.OpenShared(a, nil, workflow.Options{})
	if err != nil {
		t.Fatalf("OpenShared(%v) error = %v", a, err)
	}
	if w == nil {
		t.Fatal("OpenShared returned nil Workflow")
	}
}

func TestRegisterShared_nilFactory(t *testing.T) {
	t.Parallel()

	a := freshWorkflowAdapter()
	if err := workflow.RegisterShared(a, nil); !errors.Is(err, workflow.ErrNilFactory) {
		t.Fatalf("RegisterShared(nil) err = %v, want ErrNilFactory", err)
	}
}

func TestRegisterShared_duplicate(t *testing.T) {
	t.Parallel()

	a := freshWorkflowAdapter()
	stub := func(coredb.DB, workflow.Options) (workflow.Workflow, error) { return stubWorkflow{}, nil }
	if err := workflow.RegisterShared(a, stub); err != nil {
		t.Fatalf("first RegisterShared(%v) error = %v", a, err)
	}
	if err := workflow.RegisterShared(a, stub); !errors.Is(err, workflow.ErrDuplicate) {
		t.Fatalf("second RegisterShared(%v) err = %v, want ErrDuplicate", a, err)
	}
}

func TestOpenShared_unknown(t *testing.T) {
	t.Parallel()

	_, err := workflow.OpenShared(freshWorkflowAdapter(), nil, workflow.Options{})
	if !errors.Is(err, workflow.ErrUnknownAdapter) {
		t.Fatalf("OpenShared(unknown) err = %v, want ErrUnknownAdapter", err)
	}
}

func TestOpenShared_invalidOptions(t *testing.T) {
	t.Parallel()

	a := freshWorkflowAdapter()
	if err := workflow.RegisterShared(a, func(coredb.DB, workflow.Options) (workflow.Workflow, error) {
		return stubWorkflow{}, nil
	}); err != nil {
		t.Fatalf("RegisterShared(%v) error = %v", a, err)
	}

	_, err := workflow.OpenShared(a, nil, workflow.Options{HostPort: "missing-port"})
	if !errors.Is(err, workflow.ErrInvalidOptions) {
		t.Fatalf("OpenShared(invalid opts) err = %v, want ErrInvalidOptions", err)
	}
}

func TestOpenShared_factoryError_wrapped(t *testing.T) {
	t.Parallel()

	a := freshWorkflowAdapter()
	sentinel := errors.New("shared boom")
	if err := workflow.RegisterShared(a, func(coredb.DB, workflow.Options) (workflow.Workflow, error) {
		return nil, sentinel
	}); err != nil {
		t.Fatalf("RegisterShared(%v) error = %v", a, err)
	}

	_, err := workflow.OpenShared(a, nil, workflow.Options{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("OpenShared err = %v, want wrap of sentinel", err)
	}
	if !strings.Contains(err.Error(), "workflow: open shared") {
		t.Fatalf("OpenShared err %q missing %q", err.Error(), "workflow: open shared")
	}
}

func TestOptionsValidate_hostPort_boundaries(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		host    string
		wantErr bool
	}{
		{"empty", "", false},
		{"host port", "localhost:7233", false},
		{"ipv6", "[::1]:7233", false},
		{"missing port", "localhost", true},
		{"too many colons", "a:b:c", true},
		{"port only", ":7233", false},
		{"non-numeric port accepted by SplitHostPort", "localhost:notaport", false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := (workflow.Options{HostPort: tc.host}).Validate()
			if tc.wantErr && !errors.Is(err, workflow.ErrInvalidOptions) {
				t.Fatalf("Validate(%q) err = %v, want ErrInvalidOptions", tc.host, err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("Validate(%q) err = %v, want nil", tc.host, err)
			}
		})
	}
}

func TestRegister_Open_concurrent(t *testing.T) {
	t.Parallel()

	const n = 16
	var wg sync.WaitGroup
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()

			a := freshWorkflowAdapter()
			if err := workflow.Register(a, func(workflow.Options) (workflow.Workflow, error) {
				return stubWorkflow{}, nil
			}); err != nil {
				t.Errorf("Register(%v) error = %v", a, err)
				return
			}
			if _, err := workflow.Open(a, workflow.Options{}); err != nil {
				t.Errorf("Open(%v) error = %v", a, err)
			}
		}()
	}

	wg.Wait()
}
