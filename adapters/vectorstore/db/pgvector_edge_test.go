package db

import (
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/core/vectorstore"
)

// TestSQLiteLeg_concurrentUpsertQuery exercises upserts and brute-force
// queries from many goroutines against one store; the race detector enforces
// the dimension-lock and connection-pool correctness.
func TestSQLiteLeg_concurrentUpsertQuery(t *testing.T) {
	t.Parallel()

	s, err := New(vectorstore.Options{DSN: filepath.Join(t.TempDir(), "vectors.db")})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	t.Cleanup(func() { _ = s.Close() })

	ctx := t.Context()
	emb := []float32{0.1, 0.2, 0.3, 0.4}

	var wg sync.WaitGroup

	for i := 0; i < 16; i++ {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			vec := vectorstore.Vector{ID: "c-" + strconv.Itoa(i), Embedding: emb}

			if uerr := s.Upsert(ctx, vec); uerr != nil {
				t.Errorf("Upsert err = %v, want nil", uerr)
				return
			}

			if _, qerr := s.Query(ctx, emb, 5); qerr != nil {
				t.Errorf("Query err = %v, want nil", qerr)
			}
		}(i)
	}

	wg.Wait()
}
