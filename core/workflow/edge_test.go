package workflow_test

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/workflow"
)







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

