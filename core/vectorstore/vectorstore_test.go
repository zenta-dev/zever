package vectorstore

import (
	"context"
	"sync/atomic"
)

var testSeq atomic.Int64

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
		return NotFoundError{ID: id}
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
