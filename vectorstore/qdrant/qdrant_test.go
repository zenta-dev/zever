package qdrant

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/qdrant/go-client/qdrant"

	"github.com/zenta-dev/zever/vectorstore"
)

type stored struct {
	vec     []float32
	payload map[string]*qdrant.Value
}

type fakeClient struct {
	mu               sync.Mutex
	points           map[string]stored
	exists           bool
	dim              int
	noParams         bool
	nilInfo          bool
	existsErr        error
	recheckErr       error
	recheckSet       bool
	createErr        error
	createSetsExists bool
	infoErr          error
	upsertErr        error
	deleteErr        error
	queryErr         error
	closeErr         error
	closed           bool
	creates          int
}

func newFake() *fakeClient {
	return &fakeClient{points: make(map[string]stored)}
}

func (f *fakeClient) Upsert(_ context.Context, req *qdrant.UpsertPoints) (*qdrant.UpdateResult, error) {
	if f.upsertErr != nil {
		return nil, f.upsertErr
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	for _, p := range req.Points {
		key := p.Id.GetUuid()
		vec := append([]float32(nil), p.Vectors.GetVector().GetDense().GetData()...)
		f.points[key] = stored{vec: vec, payload: p.Payload}
	}

	return &qdrant.UpdateResult{}, nil
}

func (f *fakeClient) Delete(_ context.Context, req *qdrant.DeletePoints) (*qdrant.UpdateResult, error) {
	if f.deleteErr != nil {
		return nil, f.deleteErr
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	sel, _ := req.Points.PointsSelectorOneOf.(*qdrant.PointsSelector_Points)
	if sel != nil {
		for _, id := range sel.Points.Ids {
			delete(f.points, id.GetUuid())
		}
	}

	return &qdrant.UpdateResult{}, nil
}

func cosine(a, b []float32) float32 {
	var dot, na, nb float64

	for i := range a {
		dot += float64(a[i] * b[i])
		na += float64(a[i] * a[i])
		nb += float64(b[i] * b[i])
	}

	if na == 0 || nb == 0 {
		return 0
	}

	return float32(dot / (math.Sqrt(na) * math.Sqrt(nb)))
}

func (f *fakeClient) Query(_ context.Context, req *qdrant.QueryPoints) ([]*qdrant.ScoredPoint, error) {
	if f.queryErr != nil {
		return nil, f.queryErr
	}

	q := req.Query.GetNearest().GetDense().GetData()
	limit := 10
	if req.Limit != nil {
		limit = int(*req.Limit) //nolint:gosec // G115: test-only limit fits in int
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	out := make([]*qdrant.ScoredPoint, 0, len(f.points))
	for key, s := range f.points {
		out = append(out, &qdrant.ScoredPoint{
			Id:      qdrant.NewID(key),
			Score:   cosine(q, s.vec),
			Payload: s.payload,
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })

	if len(out) > limit {
		out = out[:limit]
	}

	return out, nil
}

func (f *fakeClient) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.closed = true

	return f.closeErr
}

func (f *fakeClient) CollectionExists(_ context.Context, _ string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.recheckSet {
		f.recheckSet = false
		return f.exists, f.recheckErr
	}

	return f.exists, f.existsErr
}

func (f *fakeClient) CreateCollection(_ context.Context, req *qdrant.CreateCollection) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.creates++

	if f.createSetsExists {
		f.exists = true
	}

	if f.createErr != nil {
		return f.createErr
	}

	f.exists = true
	f.dim = int(req.VectorsConfig.GetParams().GetSize()) //nolint:gosec // G115: test-only dimension fits in int

	return nil
}

func (f *fakeClient) GetCollectionInfo(_ context.Context, _ string) (*qdrant.CollectionInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.infoErr != nil {
		return nil, f.infoErr
	}

	if f.nilInfo {
		var info *qdrant.CollectionInfo
		return info, nil
	}

	params := &qdrant.CollectionParams{}
	if !f.noParams {
		params.VectorsConfig = qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     uint64(f.dim), //nolint:gosec // G115: test-only conversion
			Distance: qdrant.Distance_Cosine,
		})
	}

	return &qdrant.CollectionInfo{
		Config: &qdrant.CollectionConfig{Params: params},
	}, nil
}

func newStore(f *fakeClient, dim int) *Store {
	return &Store{client: f, dim: dim}
}

func TestUpsertQueryRoundtrip(t *testing.T) {
	t.Parallel()

	f := newFake()
	s := newStore(f, 0)
	ctx := context.Background()

	if err := s.Upsert(ctx, vectorstore.Vector{ID: "a", Embedding: []float32{1, 0}}); err != nil {
		t.Fatalf("upsert a: %v", err)
	}

	if err := s.Upsert(ctx, vectorstore.Vector{ID: "b", Embedding: []float32{0, 1}}); err != nil {
		t.Fatalf("upsert b: %v", err)
	}

	got, err := s.Query(ctx, []float32{1, 0}, 2)
	if err != nil {
		t.Fatalf("query: %v", err)
	}

	if len(got) != 2 || got[0].ID != "a" {
		t.Fatalf("identical vector not first: %+v", got)
	}
}

func TestUpsertBatch_MatchesLoopedUpsert(t *testing.T) {
	t.Parallel()

	vecs := []vectorstore.Vector{
		{ID: "a", Embedding: []float32{1, 0}, Metadata: map[string]any{"n": "one"}},
		{ID: "b", Embedding: []float32{0, 1}, Metadata: map[string]any{"n": "two"}},
		{ID: "c", Embedding: []float32{1, 1}, Metadata: map[string]any{"n": "three"}},
	}

	loopFake := newFake()
	loopStore := newStore(loopFake, 0)
	ctx := context.Background()

	for _, v := range vecs {
		if err := loopStore.Upsert(ctx, v); err != nil {
			t.Fatalf("loop upsert %s: %v", v.ID, err)
		}
	}

	batchFake := newFake()
	batchStore := newStore(batchFake, 0)

	if err := batchStore.UpsertBatch(ctx, vecs); err != nil {
		t.Fatalf("upsert batch: %v", err)
	}

	loopGot, err := loopStore.Query(ctx, []float32{1, 0}, 3)
	if err != nil {
		t.Fatalf("loop query: %v", err)
	}

	batchGot, err := batchStore.Query(ctx, []float32{1, 0}, 3)
	if err != nil {
		t.Fatalf("batch query: %v", err)
	}

	if len(loopGot) != len(batchGot) {
		t.Fatalf("result count mismatch: loop=%d batch=%d", len(loopGot), len(batchGot))
	}

	for i := range loopGot {
		if loopGot[i].ID != batchGot[i].ID {
			t.Fatalf("id mismatch at %d: loop=%s batch=%s", i, loopGot[i].ID, batchGot[i].ID)
		}

		if loopGot[i].Metadata["n"] != batchGot[i].Metadata["n"] {
			t.Fatalf("metadata mismatch at %d", i)
		}
	}

	// UpsertBatch is a single native batch call: exactly one Upsert RPC
	// carrying all points, unlike the loop's N separate calls.
	if batchFake.creates != loopFake.creates {
		t.Fatalf("collection create count mismatch: loop=%d batch=%d", loopFake.creates, batchFake.creates)
	}
}

func TestUpsertBatch_Empty(t *testing.T) {
	t.Parallel()

	f := newFake()
	s := newStore(f, 3)

	if err := s.UpsertBatch(context.Background(), nil); err != nil {
		t.Fatalf("UpsertBatch(nil) = %v, want nil", err)
	}
}

func TestUpsertBatch_EmptyEmbedding(t *testing.T) {
	t.Parallel()

	f := newFake()
	s := newStore(f, 0)

	err := s.UpsertBatch(context.Background(), []vectorstore.Vector{
		{ID: "a", Embedding: []float32{1, 0}},
		{ID: "b", Embedding: nil},
	})
	if !errors.Is(err, vectorstore.ErrEmptyEmbedding) {
		t.Fatalf("UpsertBatch() = %v, want ErrEmptyEmbedding", err)
	}
}

func TestUpsertBatch_DimensionMismatch(t *testing.T) {
	t.Parallel()

	f := newFake()
	s := newStore(f, 2)

	err := s.UpsertBatch(context.Background(), []vectorstore.Vector{
		{ID: "a", Embedding: []float32{1, 0}},
		{ID: "b", Embedding: []float32{1, 0, 0}},
	})

	var mismatch *vectorstore.DimensionMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("UpsertBatch() = %v, want DimensionMismatchError", err)
	}
}

func TestDeleteIdempotent(t *testing.T) {
	t.Parallel()

	f := newFake()
	s := newStore(f, 0)
	ctx := context.Background()

	if err := s.Upsert(ctx, vectorstore.Vector{ID: "gone", Embedding: []float32{1, 2}}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	if err := s.Delete(ctx, "gone"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if err := s.Delete(ctx, "missing"); err != nil {
		t.Fatalf("missing delete not nil: %v", err)
	}
}

func TestNonUUIDHashedAndRestored(t *testing.T) {
	t.Parallel()

	f := newFake()
	s := newStore(f, 0)
	ctx := context.Background()

	if err := s.Upsert(ctx, vectorstore.Vector{ID: "not-a-uuid", Embedding: []float32{1, 0}}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := s.Query(ctx, []float32{1, 0}, 1)
	if err != nil {
		t.Fatalf("query: %v", err)
	}

	if len(got) != 1 || got[0].ID != "not-a-uuid" {
		t.Fatalf("original id not restored: %+v", got)
	}

	if _, ok := got[0].Metadata["_id"]; ok {
		t.Fatalf("_id leaked into metadata: %+v", got[0].Metadata)
	}
}

func TestValidUUIDUnchanged(t *testing.T) {
	t.Parallel()

	id := "123e4567-e89b-12d3-a456-426614174000"
	f := newFake()
	s := newStore(f, 0)
	ctx := context.Background()

	if err := s.Upsert(ctx, vectorstore.Vector{ID: id, Embedding: []float32{1, 0}}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := s.Query(ctx, []float32{1, 0}, 1)
	if err != nil {
		t.Fatalf("query: %v", err)
	}

	if len(got) != 1 || got[0].ID != id {
		t.Fatalf("uuid changed: %+v", got)
	}
}

func TestPointIDDeterminism(t *testing.T) {
	t.Parallel()

	a1 := pointID("alpha").GetUuid()
	a2 := pointID("alpha").GetUuid()
	b := pointID("beta").GetUuid()

	if a1 != a2 {
		t.Fatalf("pointID not deterministic: %q vs %q", a1, a2)
	}

	if a1 == b {
		t.Fatalf("distinct inputs collided: %q", a1)
	}

	u := "123e4567-e89b-12d3-a456-426614174000"
	if got := pointID(u).GetUuid(); got != u {
		t.Fatalf("uuid passthrough changed: %q", got)
	}
}

func TestMetadataRoundtrip(t *testing.T) {
	t.Parallel()

	f := newFake()
	s := newStore(f, 0)
	ctx := context.Background()

	md := map[string]any{
		"str":   "v",
		"num":   1.5,
		"truth": true,
		"list":  []any{1.0, "x", false},
		"nest":  map[string]any{"k": "v"},
		"nada":  nil,
	}

	if err := s.Upsert(ctx, vectorstore.Vector{ID: "m", Embedding: []float32{1, 0}, Metadata: md}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := s.Query(ctx, []float32{1, 0}, 1)
	if err != nil {
		t.Fatalf("query: %v", err)
	}

	back := got[0].Metadata
	if back["str"] != "v" || back["truth"] != true || back["nada"] != nil {
		t.Fatalf("scalar metadata lost: %+v", back)
	}

	nest, ok := back["nest"].(map[string]any)
	if !ok || nest["k"] != "v" {
		t.Fatalf("nested metadata lost: %+v", back)
	}

	list, ok := back["list"].([]any)
	if !ok || len(list) != 3 {
		t.Fatalf("list metadata lost: %+v", back)
	}
}

func TestUpsertRejectsBadMetadata(t *testing.T) {
	t.Parallel()

	f := newFake()
	s := newStore(f, 0)

	err := s.Upsert(context.Background(), vectorstore.Vector{
		ID:        "bad",
		Embedding: []float32{1},
		Metadata:  map[string]any{"fn": func() {}},
	})
	if err == nil || !strings.Contains(err.Error(), "metadata") {
		t.Fatalf("expected metadata error, got %v", err)
	}
}

func TestUpsertErrors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("empty embedding", func(t *testing.T) {
		t.Parallel()

		s := newStore(newFake(), 0)
		if err := s.Upsert(ctx, vectorstore.Vector{ID: "x"}); !errors.Is(err, vectorstore.ErrEmptyEmbedding) {
			t.Fatalf("expected ErrEmptyEmbedding, got %v", err)
		}
	})

	t.Run("dimension mismatch", func(t *testing.T) {
		t.Parallel()

		s := newStore(newFake(), 3)
		err := s.Upsert(ctx, vectorstore.Vector{ID: "x", Embedding: []float32{1, 2}})
		var dm *vectorstore.DimensionMismatchError

		if !errors.As(err, &dm) || dm.Got != 2 || dm.Want != 3 {
			t.Fatalf("expected DimensionMismatchError{2,3}, got %v", err)
		}
	})

	t.Run("ensure error", func(t *testing.T) {
		t.Parallel()

		f := newFake()
		f.existsErr = errors.New("boom")
		s := newStore(f, 0)

		if err := s.Upsert(ctx, vectorstore.Vector{ID: "x", Embedding: []float32{1}}); err == nil {
			t.Fatalf("expected ensure error")
		}
	})

	t.Run("client error", func(t *testing.T) {
		t.Parallel()

		f := newFake()
		f.upsertErr = errors.New("boom")
		s := newStore(f, 0)

		if err := s.Upsert(ctx, vectorstore.Vector{ID: "x", Embedding: []float32{1}}); err == nil {
			t.Fatalf("expected client error")
		}
	})
}

func TestDeleteError(t *testing.T) {
	t.Parallel()

	f := newFake()
	f.deleteErr = errors.New("boom")
	s := newStore(f, 0)

	if err := s.Delete(context.Background(), "x"); err == nil {
		t.Fatalf("expected delete error")
	}
}

func TestQueryErrors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("empty embedding", func(t *testing.T) {
		t.Parallel()

		s := newStore(newFake(), 0)
		if _, err := s.Query(ctx, nil, 1); !errors.Is(err, vectorstore.ErrEmptyEmbedding) {
			t.Fatalf("expected ErrEmptyEmbedding, got %v", err)
		}
	})

	t.Run("dimension mismatch", func(t *testing.T) {
		t.Parallel()

		s := newStore(newFake(), 3)
		_, err := s.Query(ctx, []float32{1, 2}, 1)
		var dm *vectorstore.DimensionMismatchError

		if !errors.As(err, &dm) || dm.Got != 2 || dm.Want != 3 {
			t.Fatalf("expected DimensionMismatchError{2,3}, got %v", err)
		}
	})

	t.Run("ensure error", func(t *testing.T) {
		t.Parallel()

		f := newFake()
		f.existsErr = errors.New("boom")
		s := newStore(f, 0)

		if _, err := s.Query(ctx, []float32{1}, 1); err == nil {
			t.Fatalf("expected ensure error")
		}
	})

	t.Run("client error", func(t *testing.T) {
		t.Parallel()

		f := newFake()
		f.queryErr = errors.New("boom")
		s := newStore(f, 0)

		if _, err := s.Query(ctx, []float32{1}, 1); err == nil {
			t.Fatalf("expected client error")
		}
	})

	t.Run("default topK", func(t *testing.T) {
		t.Parallel()

		f := newFake()
		s := newStore(f, 0)

		if err := s.Upsert(ctx, vectorstore.Vector{ID: "x", Embedding: []float32{1}}); err != nil {
			t.Fatalf("upsert: %v", err)
		}

		if _, err := s.Query(ctx, []float32{1}, 0); err != nil {
			t.Fatalf("default topK: %v", err)
		}
	})

	t.Run("lazy ensure", func(t *testing.T) {
		t.Parallel()

		f := newFake()
		s := newStore(f, 0)

		if _, err := s.Query(ctx, []float32{1, 2}, 1); err != nil {
			t.Fatalf("query: %v", err)
		}

		if !f.exists {
			t.Fatalf("query did not ensure collection")
		}
	})

	t.Run("missing _id falls back to uuid", func(t *testing.T) {
		t.Parallel()

		f := newFake()
		key := pointID("raw").GetUuid()
		f.points[key] = stored{
			vec:     []float32{1},
			payload: map[string]*qdrant.Value{},
		}
		f.exists = true
		s := newStore(f, 1)
		s.created = true

		got, err := s.Query(ctx, []float32{1}, 1)
		if err != nil {
			t.Fatalf("query: %v", err)
		}

		if len(got) != 1 || got[0].ID != key {
			t.Fatalf("expected uuid fallback %q, got %+v", key, got)
		}
	})
}

func TestEnsureCollection(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("created fast path", func(t *testing.T) {
		t.Parallel()

		f := newFake()
		s := newStore(f, 0)

		if err := s.ensureCollection(ctx, 2); err != nil {
			t.Fatalf("first ensure: %v", err)
		}

		f.existsErr = errors.New("must not be called")

		if err := s.ensureCollection(ctx, 2); err != nil {
			t.Fatalf("fast path: %v", err)
		}
	})

	t.Run("existing dimension match", func(t *testing.T) {
		t.Parallel()

		f := newFake()
		f.exists = true
		f.dim = 2
		s := newStore(f, 2)

		if err := s.ensureCollection(ctx, 2); err != nil {
			t.Fatalf("ensure: %v", err)
		}
	})

	t.Run("existing dimension mismatch", func(t *testing.T) {
		t.Parallel()

		f := newFake()
		f.exists = true
		f.dim = 4
		s := newStore(f, 2)

		if err := s.ensureCollection(ctx, 2); err == nil {
			t.Fatalf("expected dimension error")
		}
	})

	t.Run("exists check error", func(t *testing.T) {
		t.Parallel()

		f := newFake()
		f.existsErr = errors.New("boom")
		s := newStore(f, 0)

		if err := s.ensureCollection(ctx, 2); err == nil {
			t.Fatalf("expected exists error")
		}
	})

	t.Run("info error", func(t *testing.T) {
		t.Parallel()

		f := newFake()
		f.exists = true
		f.infoErr = errors.New("boom")
		s := newStore(f, 0)

		if err := s.ensureCollection(ctx, 2); err == nil {
			t.Fatalf("expected info error")
		}
	})

	t.Run("nil info", func(t *testing.T) {
		t.Parallel()

		f := newFake()
		f.exists = true
		f.nilInfo = true
		s := newStore(f, 0)

		if err := s.ensureCollection(ctx, 2); err == nil {
			t.Fatalf("expected nil-info error")
		}
	})

	t.Run("missing vector params", func(t *testing.T) {
		t.Parallel()

		f := newFake()
		f.exists = true
		f.noParams = true
		s := newStore(f, 0)

		if err := s.ensureCollection(ctx, 2); err == nil {
			t.Fatalf("expected params error")
		}
	})

	t.Run("create race recheck exists", func(t *testing.T) {
		t.Parallel()

		f := newFake()
		f.createErr = errors.New("already exists")
		f.createSetsExists = true
		s := newStore(f, 0)

		if err := s.ensureCollection(ctx, 2); err != nil {
			t.Fatalf("race recheck: %v", err)
		}
	})

	t.Run("create failure", func(t *testing.T) {
		t.Parallel()

		f := newFake()
		f.createErr = errors.New("boom")
		s := newStore(f, 0)

		if err := s.ensureCollection(ctx, 2); err == nil {
			t.Fatalf("expected create error")
		}
	})

	t.Run("create recheck error", func(t *testing.T) {
		t.Parallel()

		f := newFake()
		f.createErr = errors.New("boom")
		f.recheckSet = true
		f.recheckErr = errors.New("recheck boom")
		s := newStore(f, 0)

		if err := s.ensureCollection(ctx, 2); err == nil {
			t.Fatalf("expected recheck error")
		}
	})

	t.Run("missing params error", func(t *testing.T) {
		t.Parallel()

		f := newFake()
		f.exists = true
		f.nilInfo = true
		s := &Store{client: f, dim: 0}

		if err := s.ensureCollection(ctx, 0); err == nil {
			t.Fatalf("expected params error for zero dim")
		}
	})

	t.Run("concurrent cold start calls CreateCollection once", func(t *testing.T) {
		t.Parallel()

		f := newDelayedFake()
		s := &Store{client: f, dim: 0}

		const workers = 20

		start := make(chan struct{})

		var wg sync.WaitGroup

		errs := make([]error, workers)

		for i := range workers {
			wg.Add(1)

			go func() {
				defer wg.Done()

				<-start

				errs[i] = s.ensureCollection(ctx, 2)
			}()
		}

		close(start)
		wg.Wait()

		for i, err := range errs {
			if err != nil {
				t.Fatalf("worker %d: ensureCollection: %v", i, err)
			}
		}

		if f.creates != 1 {
			t.Errorf("CreateCollection calls = %d, want 1 (thundering herd not suppressed)", f.creates)
		}

		if got := f.existsCalls.Load(); got != 1 {
			t.Errorf("CollectionExists calls = %d, want 1 (thundering herd not suppressed)", got)
		}
	})
}

// delayedFakeClient wraps fakeClient with a small artificial delay in
// CollectionExists/CreateCollection and counts CollectionExists calls, so a
// concurrency test has a wide-enough window to prove concurrent
// ensureCollection callers are serialized instead of racing.
type delayedFakeClient struct {
	*fakeClient
	existsCalls atomic.Int64
}

func newDelayedFake() *delayedFakeClient {
	return &delayedFakeClient{fakeClient: newFake()}
}

func (f *delayedFakeClient) CollectionExists(ctx context.Context, name string) (bool, error) {
	f.existsCalls.Add(1)
	time.Sleep(5 * time.Millisecond)

	return f.fakeClient.CollectionExists(ctx, name)
}

func (f *delayedFakeClient) CreateCollection(ctx context.Context, req *qdrant.CreateCollection) error {
	time.Sleep(5 * time.Millisecond)

	return f.fakeClient.CreateCollection(ctx, req)
}

func TestParseAddr(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		addr string
		host string
		port int
		tls  bool
	}{
		{"https defaults 6335", "https://example.com", "example.com", 6335, true},
		{"http defaults 6334", "http://example.com", "example.com", 6334, false},
		{"explicit port", "http://example.com:1234", "example.com", 1234, false},
		{"https explicit port", "https://example.com:9999", "example.com", 9999, true},
		{"bad port falls back to split", "http://example.com:notaport", "http://example.com", 6334, false},
		{"empty host fallback", "http:///path", "localhost", 6334, false},
		{"bare host port", "example.com:7777", "example.com", 7777, false},
		{"bare host", "example.com", "example.com", 6334, false},
		{"bare bad port", "example.com:notaport", "example.com", 6334, false},
		{"empty", "", "localhost", 6334, false},
		{"unparseable scheme", "http://[::1", "http://[:", 1, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			host, port, tls := parseAddr(tc.addr)
			if host != tc.host || port != tc.port || tls != tc.tls {
				t.Fatalf("parseAddr(%q) = %q,%d,%v want %q,%d,%v",
					tc.addr, host, port, tls, tc.host, tc.port, tc.tls)
			}
		})
	}
}

func TestHostPort(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   *url.URL
		host string
		port int
		tls  bool
	}{
		{"http default", &url.URL{Scheme: "http", Host: "example.com"}, "example.com", 6334, false},
		{"https default", &url.URL{Scheme: "https", Host: "example.com"}, "example.com", 6335, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			host, port, tls := hostPort(tc.in)
			if host != tc.host || port != tc.port || tls != tc.tls {
				t.Fatalf("hostPort = %q,%d,%v want %q,%d,%v",
					host, port, tls, tc.host, tc.port, tc.tls)
			}
		})
	}
}

func TestRequireEmbedding(t *testing.T) {
	t.Parallel()

	if err := requireEmbedding(nil); !errors.Is(err, vectorstore.ErrEmptyEmbedding) {
		t.Fatalf("expected ErrEmptyEmbedding, got %v", err)
	}

	if err := requireEmbedding([]float32{1}); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestCollectionDim(t *testing.T) {
	t.Parallel()

	if got := collectionDim(5, 2); got != 5 {
		t.Fatalf("opt wins: got %d", got)
	}

	if got := collectionDim(0, 2); got != 2 {
		t.Fatalf("fallback: got %d", got)
	}
}

func TestExtractValue(t *testing.T) {
	t.Parallel()

	strct := &qdrant.Value{Kind: &qdrant.Value_StructValue{
		StructValue: &qdrant.Struct{Fields: map[string]*qdrant.Value{
			"k": {Kind: &qdrant.Value_StringValue{StringValue: "v"}},
		}},
	}}
	lst := &qdrant.Value{Kind: &qdrant.Value_ListValue{
		ListValue: &qdrant.ListValue{Values: []*qdrant.Value{
			{Kind: &qdrant.Value_IntegerValue{IntegerValue: 7}},
			{Kind: &qdrant.Value_NullValue{}},
		}},
	}}

	cases := []struct {
		name string
		in   *qdrant.Value
		want any
	}{
		{"string", &qdrant.Value{Kind: &qdrant.Value_StringValue{StringValue: "s"}}, "s"},
		{"double", &qdrant.Value{Kind: &qdrant.Value_DoubleValue{DoubleValue: 1.5}}, 1.5},
		{"integer", &qdrant.Value{Kind: &qdrant.Value_IntegerValue{IntegerValue: 3}}, int64(3)},
		{"bool", &qdrant.Value{Kind: &qdrant.Value_BoolValue{BoolValue: true}}, true},
		{"null", &qdrant.Value{Kind: &qdrant.Value_NullValue{}}, nil},
		{"nil kind", &qdrant.Value{}, nil},
		{"struct", strct, map[string]any{"k": "v"}},
		{"list", lst, []any{int64(7), nil}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got, want := fmt.Sprintf("%#v", extractValue(tc.in)), fmt.Sprintf("%#v", tc.want); got != want {
				t.Fatalf("extractValue = %s want %s", got, want)
			}
		})
	}
}

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("missing url", func(t *testing.T) {
		t.Parallel()

		if _, err := New(vectorstore.Options{}); !errors.Is(err, ErrMissingURL) {
			t.Fatalf("expected ErrMissingURL, got %v", err)
		}
	})

	t.Run("invalid options", func(t *testing.T) {
		t.Parallel()

		_, err := New(vectorstore.Options{URL: "http://localhost:6334", Dimension: -1})
		if err == nil {
			t.Fatalf("expected validation error")
		}
	})

	t.Run("ok lazy", func(t *testing.T) {
		t.Parallel()

		vs, err := New(vectorstore.Options{URL: "http://localhost:6334", APIKey: "secret"})
		if err != nil {
			t.Fatalf("open: %v", err)
		}

		if err := vs.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
	})

	t.Run("lazy no dim", func(t *testing.T) {
		t.Parallel()

		s, err := New(vectorstore.Options{URL: "http://localhost:6334"})
		if err != nil {
			t.Fatalf("new: %v", err)
		}

		if err := s.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
	})

	t.Run("ensure failure closes", func(t *testing.T) {
		t.Parallel()

		if _, err := New(vectorstore.Options{URL: "http://localhost:1", Dimension: 2}); err == nil {
			t.Fatalf("expected ensure error")
		}
	})

	t.Run("client error", func(t *testing.T) {
		t.Parallel()

		if _, err := New(vectorstore.Options{URL: "\x7f"}); err == nil {
			t.Fatalf("expected client error")
		}
	})
}

func TestCloseBestEffort(t *testing.T) {
	t.Parallel()

	f := newFake()
	f.closeErr = errors.New("boom")
	s := newStore(f, 0)

	if err := s.Close(); err != nil {
		t.Fatalf("close must be nil, got %v", err)
	}

	if !f.closed {
		t.Fatalf("underlying close not called")
	}
}

func TestConcurrentUpsertQuery(t *testing.T) {
	t.Parallel()

	f := newFake()
	s := newStore(f, 0)
	ctx := context.Background()

	var wg sync.WaitGroup

	for i := range 8 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			id := fmt.Sprintf("id-%d", i)
			_ = s.Upsert(ctx, vectorstore.Vector{ID: id, Embedding: []float32{float32(i), 1}})

			_, _ = s.Query(ctx, []float32{float32(i), 1}, 3)
		}()
	}

	wg.Wait()
}
