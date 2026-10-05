package db

import (
	"path/filepath"
	"strconv"
	"strings"
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

// TestEdgeQuery_corruptMetadataBelowTopK documents the lazy metadata decode:
// metadata JSON is parsed only for rows that survive the topK heap. A row
// with corrupt metadata that ranks below topK is discarded before its
// metadata is decoded, so the query succeeds; the same row fails the query
// once topK admits it.
func TestEdgeQuery_corruptMetadataBelowTopK(t *testing.T) {
	t.Parallel()

	// File-backed, not :memory: :memory: is shared-cache, so parallel
	// tests would see each other's rows and perturb the ranking.
	s, err := New(vectorstore.Options{DSN: filepath.Join(t.TempDir(), "vectors.db")})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() { _ = s.Close() })

	ctx := t.Context()

	d, ok := s.(*driver)
	if !ok {
		t.Fatalf("New() returned %T, want *driver", s)
	}

	query := []float32{1, 0, 0, 0}

	for i := 0; i < 5; i++ {
		mustUpsert(t, s, vectorstore.Vector{
			ID:        "hit-" + strconv.Itoa(i),
			Embedding: query,
			Metadata:  map[string]any{"i": i},
		})
	}

	// Orthogonal embedding scores 0, below every hit; its corrupt metadata
	// must never be decoded while topK excludes it.
	if _, err = d.conn.Exec(ctx,
		`INSERT INTO vectors (id, embedding, metadata) VALUES (?, ?, ?)`,
		"corrupt", encodeEmbedding([]float32{0, 1, 0, 0}), []byte("{not json"),
	); err != nil {
		t.Fatalf("insert corrupt row: %v", err)
	}

	hits, err := s.Query(ctx, query, 5)

	if err != nil {
		t.Fatalf("Query(topK=5) err = %v, want nil (corrupt row below topK)", err)
	}

	if len(hits) != 5 {
		t.Fatalf("Query(topK=5) = %d hits, want 5", len(hits))
	}

	_, err = s.Query(ctx, query, 6)
	if err == nil {
		t.Fatal("Query(topK=6) = nil, want metadata decode error")
	}

	if !strings.Contains(err.Error(), "metadata decode") {
		t.Fatalf("Query(topK=6) err = %v, want metadata decode error", err)
	}
}
