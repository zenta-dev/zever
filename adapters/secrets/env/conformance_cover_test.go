package env

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/zenta-dev/zever/core/secrets"
	"github.com/zenta-dev/zever/core/secrets/secretstest"
)

var coverSeq atomic.Int64

// TestEnvConformance proves the env adapter honors the secrets.Secrets
// contract via the shared conformance kit. Each subtest gets a fresh
// prefix/window into the process environment (read-only, so Set/Delete
// report ErrNotSupported and the kit asserts the read-only branches).
// Not parallel: the factory seeds via t.Setenv, which forbids parallel
// ancestors.
func TestEnvConformance(t *testing.T) {
	secretstest.Conformance(t, func(t *testing.T) secrets.Secrets {
		t.Helper()

		prefix := fmt.Sprintf("COVER_%d_", coverSeq.Add(1))
		t.Setenv(prefix+"SEEDED", "1")

		s, err := New(Options{Prefix: prefix})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = s.Close(t.Context()) })

		return s
	})
}
