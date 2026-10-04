package vectorstore

import (
	"errors"
	"math"
	"sync"
	"testing"
)

func TestVector_Validate_table(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		meta    map[string]any
		wantErr bool
	}{
		{"nil metadata", nil, false},
		{"empty metadata", map[string]any{}, false},
		{"string value", map[string]any{"k": "v"}, false},
		{"nested", map[string]any{"k": map[string]any{"n": 1}}, false},
		{"slice", map[string]any{"k": []int{1, 2}}, false},
		{"func value", map[string]any{"k": func() {}}, true},
		{"chan value", map[string]any{"k": make(chan int)}, true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := (Vector{Metadata: tc.meta}).Validate()
			if tc.wantErr && !errors.Is(err, ErrInvalidMetadata) {
				t.Fatalf("Validate() err = %v, want ErrInvalidMetadata", err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("Validate() err = %v, want nil", err)
			}
		})
	}
}

func TestCosineSimilarity_highDimension(t *testing.T) {
	t.Parallel()

	a := benchEmbedding()
	got := CosineSimilarity(a, a)
	if math.Abs(float64(got-1)) > 1e-4 {
		t.Fatalf("self-similarity = %v, want ~1", got)
	}
}

func TestCosineSimilarity_scaleInvariant(t *testing.T) {
	t.Parallel()

	a := []float32{1, 2, 3}
	b := []float32{2, 4, 6}
	got := CosineSimilarity(a, b)
	if math.Abs(float64(got-1)) > 1e-6 {
		t.Fatalf("parallel vectors similarity = %v, want 1", got)
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

			a := freshAdapter()
			if err := Register(a, func(Options) (VectorStore, error) { return newStubStore(), nil }); err != nil {
				t.Errorf("Register(%v) error = %v", a, err)
				return
			}
			if _, err := Open(a, Options{}); err != nil {
				t.Errorf("Open(%v) error = %v", a, err)
			}
		}()
	}

	wg.Wait()
}

func TestStubStore_emptyEmbedding(t *testing.T) {
	t.Parallel()

	st := newStubStore()
	ctx := t.Context()

	if err := st.Upsert(ctx, Vector{ID: "v"}); !errors.Is(err, ErrEmptyEmbedding) {
		t.Fatalf("Upsert(empty) err = %v, want ErrEmptyEmbedding", err)
	}
	if _, err := st.Query(ctx, nil, 5); !errors.Is(err, ErrEmptyEmbedding) {
		t.Fatalf("Query(empty) err = %v, want ErrEmptyEmbedding", err)
	}
	if err := st.Delete(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete(missing) err = %v, want ErrNotFound", err)
	}
}
