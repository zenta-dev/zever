package rag

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/ai"
	"github.com/zenta-dev/zever/core/vectorstore"
)

type fakeAI struct {
	embeds     [][]float32
	embedErr   error
	gen        ai.Generation
	genErr     error
	embedCalls int
	lastMsgs   []ai.Message
}

func (f *fakeAI) Generate(_ context.Context, _ string, messages []ai.Message, _ ai.GenerateOptions) (ai.Generation, error) {
	f.lastMsgs = messages
	return f.gen, f.genErr
}

func (f *fakeAI) Stream(context.Context, string, []ai.Message, ai.GenerateOptions) (<-chan ai.StreamChunk, error) {
	ch := make(chan ai.StreamChunk)
	close(ch)
	return ch, nil
}

func (f *fakeAI) Embed(_ context.Context, _ string, inputs []string, _ ai.EmbedOptions) ([][]float32, error) {
	f.embedCalls++
	if f.embedErr != nil {
		return nil, f.embedErr
	}
	if f.embeds != nil {
		return f.embeds, nil
	}

	out := make([][]float32, len(inputs))
	for i := range inputs {
		out[i] = []float32{float32(i), 1}
	}
	return out, nil
}

func (f *fakeAI) Close() error { return nil }

type memStore struct {
	vecs       []vectorstore.Vector
	batchCalls int
}

func (m *memStore) Upsert(_ context.Context, v vectorstore.Vector) error {
	m.vecs = append(m.vecs, v)
	return nil
}

func (m *memStore) UpsertBatch(_ context.Context, vs []vectorstore.Vector) error {
	m.batchCalls++
	m.vecs = append(m.vecs, vs...)
	return nil
}

func (m *memStore) Delete(context.Context, string) error { return nil }

func (m *memStore) Query(_ context.Context, _ []float32, topK int) ([]vectorstore.ScoreMatch, error) {
	out := make([]vectorstore.ScoreMatch, 0, len(m.vecs))
	for i, v := range m.vecs {
		out = append(out, vectorstore.ScoreMatch{ID: v.ID, Score: float32(len(m.vecs) - i), Metadata: v.Metadata})
	}
	if topK > 0 && len(out) > topK {
		out = out[:topK]
	}
	return out, nil
}

func (m *memStore) Close() error { return nil }

func newTestEngine(t *testing.T, f *fakeAI, s *memStore, opts Options) *Engine {
	t.Helper()

	e, err := New(f, s, opts)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return e
}

func TestNew_validation(t *testing.T) {
	t.Parallel()

	if _, err := New(nil, &memStore{}, Options{}); !errors.Is(err, ErrNilClient) {
		t.Fatalf("New(nil client) error = %v, want ErrNilClient", err)
	}
	if _, err := New(&fakeAI{}, nil, Options{}); !errors.Is(err, ErrNilStore) {
		t.Fatalf("New(nil store) error = %v, want ErrNilStore", err)
	}
	if _, err := New(&fakeAI{}, &memStore{}, Options{ChunkSize: -1}); !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("New(negative chunk) error = %v, want ErrInvalidOptions", err)
	}
}

func TestChunkText(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		text    string
		size    int
		overlap int
		want    []string
	}{
		{"empty", "", 5, 2, nil},
		{"single", "abc", 5, 2, []string{"abc"}},
		{"windows", "abcdefghij", 5, 2, []string{"abcde", "defgh", "ghij"}},
		{"overlap clamped", "abcdefghij", 5, 9, []string{"abcde", "defgh", "ghij"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := chunkText(tc.text, tc.size, tc.overlap)
			if len(got) != len(tc.want) {
				t.Fatalf("chunkText() = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("chunkText()[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestIngest_embedsAndUpserts(t *testing.T) {
	t.Parallel()

	f := &fakeAI{}
	s := &memStore{}
	e := newTestEngine(t, f, s, Options{ChunkSize: 5, ChunkOverlap: 0})

	err := e.Ingest(t.Context(), []Document{
		{ID: "d1", Content: "abcdefghij", Metadata: map[string]any{"lang": "en"}},
	})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}

	if len(s.vecs) != 2 {
		t.Fatalf("stored %d vectors, want 2", len(s.vecs))
	}
	if s.batchCalls != 1 {
		t.Errorf("batch calls = %d, want 1", s.batchCalls)
	}
	if s.vecs[0].Metadata["content"] != "abcde" {
		t.Errorf("chunk content = %v, want abcde", s.vecs[0].Metadata["content"])
	}
	if s.vecs[0].Metadata["lang"] != "en" {
		t.Errorf("document metadata not copied: %v", s.vecs[0].Metadata)
	}
	if f.embedCalls != 1 {
		t.Errorf("embed calls = %d, want 1", f.embedCalls)
	}
}

func TestIngest_skipsEmptyContent(t *testing.T) {
	t.Parallel()

	f := &fakeAI{}
	s := &memStore{}
	e := newTestEngine(t, f, s, Options{})

	if err := e.Ingest(t.Context(), []Document{{ID: "empty"}}); err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if len(s.vecs) != 0 || f.embedCalls != 0 {
		t.Fatalf("expected no work, got %d vecs %d embeds", len(s.vecs), f.embedCalls)
	}
}

func TestIngest_invalidDocument(t *testing.T) {
	t.Parallel()

	e := newTestEngine(t, &fakeAI{}, &memStore{}, Options{})

	if err := e.Ingest(t.Context(), []Document{{Content: "x"}}); !errors.Is(err, ErrInvalidDocument) {
		t.Fatalf("Ingest() error = %v, want ErrInvalidDocument", err)
	}
}

func TestIngest_embedCountMismatch(t *testing.T) {
	t.Parallel()

	f := &fakeAI{embeds: [][]float32{{1}}}
	e := newTestEngine(t, f, &memStore{}, Options{ChunkSize: 5})

	err := e.Ingest(t.Context(), []Document{{ID: "d", Content: "abcdefghij"}})
	if !errors.Is(err, ErrEmbeddingCount) {
		t.Fatalf("Ingest() error = %v, want ErrEmbeddingCount", err)
	}
}

func TestIngest_embedError(t *testing.T) {
	t.Parallel()

	want := errors.New("embed down")
	e := newTestEngine(t, &fakeAI{embedErr: want}, &memStore{}, Options{})

	if err := e.Ingest(t.Context(), []Document{{ID: "d", Content: "hello"}}); !errors.Is(err, want) {
		t.Fatalf("Ingest() error = %v, want %v", err, want)
	}
}

func TestRetrieve(t *testing.T) {
	t.Parallel()

	f := &fakeAI{}
	s := &memStore{vecs: []vectorstore.Vector{
		{ID: "a#0", Metadata: map[string]any{"content": "first"}},
		{ID: "b#0", Metadata: map[string]any{"content": "second"}},
	}}
	e := newTestEngine(t, f, s, Options{TopK: 1})

	sources, err := e.Retrieve(t.Context(), "query", 0)
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("sources = %d, want 1 (TopK)", len(sources))
	}
	if sources[0].Content != "first" {
		t.Errorf("source content = %q, want first", sources[0].Content)
	}
}

func TestRetrieve_emptyQuery(t *testing.T) {
	t.Parallel()

	e := newTestEngine(t, &fakeAI{}, &memStore{}, Options{})

	if _, err := e.Retrieve(t.Context(), "", 3); !errors.Is(err, ErrEmptyQuery) {
		t.Fatalf("Retrieve() error = %v, want ErrEmptyQuery", err)
	}
}

func TestAnswer_groundsAndReturnsSources(t *testing.T) {
	t.Parallel()

	f := &fakeAI{gen: ai.Generation{Content: "grounded answer", Usage: ai.Usage{CompletionTokens: 3}}}
	s := &memStore{vecs: []vectorstore.Vector{
		{ID: "a#0", Metadata: map[string]any{"content": "context one"}},
	}}
	e := newTestEngine(t, f, s, Options{Model: "m"})

	ans, err := e.Answer(t.Context(), "what?", 0)
	if err != nil {
		t.Fatalf("Answer() error = %v", err)
	}
	if ans.Text != "grounded answer" {
		t.Errorf("answer text = %q, want grounded answer", ans.Text)
	}
	if len(ans.Sources) != 1 {
		t.Fatalf("sources = %d, want 1", len(ans.Sources))
	}
	if ans.Usage.CompletionTokens != 3 {
		t.Errorf("usage = %+v, want completion 3", ans.Usage)
	}

	var user string
	for _, m := range f.lastMsgs {
		if m.Role == ai.RoleUser {
			user = m.Content
		}
	}
	if !strings.Contains(user, "context one") {
		t.Errorf("user prompt missing retrieved context: %q", user)
	}
}
