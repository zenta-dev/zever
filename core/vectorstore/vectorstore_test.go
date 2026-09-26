package vectorstore

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
)

var testSeq atomic.Int64

func freshAdapter() Adapter {
	return Adapter(1000 + int(testSeq.Add(1)))
}

type stubStore struct {
	vecs map[string]Vector
}

func newStubStore() *stubStore {
	return &stubStore{vecs: make(map[string]Vector)}
}

func (s *stubStore) Upsert(_ context.Context, vec Vector) error {
	if len(vec.Embedding) == 0 {
		return ErrEmptyEmbedding
	}

	s.vecs[vec.ID] = vec

	return nil
}

func (s *stubStore) UpsertBatch(ctx context.Context, vecs []Vector) error {
	for _, vec := range vecs {
		if err := s.Upsert(ctx, vec); err != nil {
			return err
		}
	}

	return nil
}

func (s *stubStore) Delete(_ context.Context, id string) error {
	if _, ok := s.vecs[id]; !ok {
		return &NotFoundError{ID: id}
	}

	delete(s.vecs, id)

	return nil
}

func (s *stubStore) Query(_ context.Context, embedding []float32, topK int) ([]ScoreMatch, error) {
	if len(embedding) == 0 {
		return nil, ErrEmptyEmbedding
	}

	if topK <= 0 {
		topK = DefaultTopK
	}

	var out []ScoreMatch

	for _, v := range s.vecs {
		out = append(out, ScoreMatch{ID: v.ID, Score: CosineSimilarity(embedding, v.Embedding), Metadata: v.Metadata})
		if len(out) >= topK {
			break
		}
	}

	return out, nil
}

func (s *stubStore) Close() error {
	return nil
}

func TestRegister_nilFactory_returnsNilFactory(t *testing.T) {
	a := freshAdapter()
	if err := Register(a, nil); !errors.Is(err, ErrNilFactory) {
		t.Fatalf("Register nil err = %v, want ErrNilFactory", err)
	}
}

func TestRegister_duplicate_returnsDuplicateError(t *testing.T) {
	a := freshAdapter()
	ok := func(Options) (VectorStore, error) { return newStubStore(), nil }
	if err := Register(a, ok); err != nil {
		t.Fatalf("first Register err = %v", err)
	}

	if err := Register(a, ok); !errors.Is(err, ErrDuplicateAdapter) {
		t.Fatalf("second Register err = %v, want ErrDuplicateAdapter", err)
	}

	var de *DuplicateAdapterError

	if err := Register(a, ok); !errors.As(err, &de) {
		t.Fatalf("dup err %T is not *DuplicateAdapterError", err)
	} else if de.Adapter != a {
		t.Fatalf("dup carried adapter = %v, want %v", de.Adapter, a)
	}
}

func TestOpen_unknownAdapter_returnsUnknownAndNil(t *testing.T) {
	a := Adapter(9999)

	got, err := Open(a, Options{})
	if !errors.Is(err, ErrUnknownAdapter) {
		t.Fatalf("Open unknown err = %v, want ErrUnknownAdapter", err)
	}

	var ue *UnknownAdapterError
	if !errors.As(err, &ue) {
		t.Fatalf("err %T is not *UnknownAdapterError", err)
	}

	if ue.Adapter != a {
		t.Fatalf("carried adapter = %v, want %v", ue.Adapter, a)
	}

	if got != nil {
		t.Fatalf("Open unknown value = %v, want nil", got)
	}
}

func TestOpen_factoryError_wrappedWithPrefixAndNil(t *testing.T) {
	a := freshAdapter()
	sentinel := errors.New("boom")

	if err := Register(a, func(Options) (VectorStore, error) { return nil, sentinel }); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	got, err := Open(a, Options{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Open err = %v, want wrap of sentinel", err)
	}

	if !strings.Contains(err.Error(), "vectorstore: open") {
		t.Fatalf("Open err %q missing %q", err.Error(), "vectorstore: open")
	}

	if got != nil {
		t.Fatalf("Open error value = %v, want nil", got)
	}
}

func TestOpen_success_smoke(t *testing.T) {
	a := freshAdapter()
	if err := Register(a, func(Options) (VectorStore, error) { return newStubStore(), nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	got, err := Open(a, Options{})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}

	ctx := t.Context()

	if upErr := got.Upsert(ctx, Vector{ID: "v1", Embedding: []float32{1, 0}}); upErr != nil {
		t.Fatalf("Upsert err = %v", upErr)
	}

	matches, err := got.Query(ctx, []float32{1, 0}, 0)
	if err != nil {
		t.Fatalf("Query err = %v", err)
	}

	if len(matches) != 1 || matches[0].ID != "v1" {
		t.Fatalf("Query = %v, want one match v1", matches)
	}

	if delErr := got.Delete(ctx, "v1"); delErr != nil {
		t.Fatalf("Delete err = %v", delErr)
	}

	if closeErr := got.Close(); closeErr != nil {
		t.Fatalf("Close err = %v", closeErr)
	}
}

func TestOpen_invalidOptions_propagatesInvalidOptions(t *testing.T) {
	a := freshAdapter()
	if err := Register(a, func(Options) (VectorStore, error) { return newStubStore(), nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	got, err := Open(a, Options{Dimension: -1})
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("Open err = %v, want ErrInvalidOptions", err)
	}

	var ioe *InvalidOptionsError
	if !errors.As(err, &ioe) {
		t.Fatalf("err %T is not *InvalidOptionsError", err)
	}

	if got != nil {
		t.Fatalf("Open error value = %v, want nil", got)
	}
}
