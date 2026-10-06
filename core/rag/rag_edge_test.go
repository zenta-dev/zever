package rag

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/core/ai"
	"github.com/zenta-dev/zever/core/vectorstore"
)

// stubErrStore fails UpsertBatch with a fixed error.
type stubErrStore struct {
	memStore
	err error
}

func (s *stubErrStore) UpsertBatch(context.Context, []vectorstore.Vector) error { return s.err }

// stubCtxStore honors context cancellation.
type stubCtxStore struct{ memStore }

func (s *stubCtxStore) UpsertBatch(ctx context.Context, vs []vectorstore.Vector) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.vecs = append(s.vecs, vs...)
	return nil
}

func (s *stubCtxStore) Query(ctx context.Context, _ []float32, topK int) ([]vectorstore.ScoreMatch, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.memStore.Query(ctx, nil, topK)
}

// stubCtxAI honors context cancellation on Embed.
type stubCtxAI struct{ fakeAI }

func (s *stubCtxAI) Embed(ctx context.Context, _ string, inputs []string, _ ai.EmbedOptions) ([][]float32, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.fakeAI.Embed(ctx, "", inputs, ai.EmbedOptions{})
}

// stubQueryErrStore fails Query with a fixed error.
type stubQueryErrStore struct {
	memStore
	err error
}

func (s *stubQueryErrStore) Query(context.Context, []float32, int) ([]vectorstore.ScoreMatch, error) {
	return nil, s.err
}

func TestEdgeDocumentValidate(t *testing.T) {
	t.Parallel()

	if err := (Document{ID: "d", Content: ""}).Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil for empty content", err)
	}
	if err := (Document{ID: "d", Content: "x"}).Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}

	err := (Document{}).Validate()
	if !errors.Is(err, ErrInvalidDocument) {
		t.Fatalf("Validate() error = %v, want ErrInvalidDocument", err)
	}
	var invalid InvalidDocumentError
	if !errors.As(err, &invalid) {
		t.Fatalf("error %T is not InvalidDocumentError", err)
	}
}

func TestEdgeWithDefaults(t *testing.T) {
	t.Parallel()

	got := (Options{}).withDefaults()
	// Zero ChunkOverlap is a valid explicit value; only negative selects default.
	if got.TopK != DefaultTopK || got.ChunkSize != DefaultChunkSize || got.ChunkOverlap != 0 {
		t.Fatalf("withDefaults() = %+v, want TopK %d ChunkSize %d overlap 0", got, DefaultTopK, DefaultChunkSize)
	}

	kept := (Options{TopK: 2, ChunkSize: 10, ChunkOverlap: 0}).withDefaults()
	if kept.TopK != 2 || kept.ChunkSize != 10 || kept.ChunkOverlap != 0 {
		t.Fatalf("withDefaults() = %+v, want values kept", kept)
	}

	// Negative overlap selects the default; zero overlap is a valid explicit value.
	if got := (Options{ChunkOverlap: -1}).withDefaults().ChunkOverlap; got != DefaultChunkOverlap {
		t.Fatalf("ChunkOverlap = %d, want %d", got, DefaultChunkOverlap)
	}
}

func TestEdgeEmbedModel(t *testing.T) {
	t.Parallel()

	if got := (Options{Model: "gen"}).embedModel(); got != "gen" {
		t.Fatalf("embedModel() = %q, want gen fallback", got)
	}
	if got := (Options{Model: "gen", EmbedModel: "emb"}).embedModel(); got != "emb" {
		t.Fatalf("embedModel() = %q, want emb", got)
	}
	if got := (Options{}).embedModel(); got != "" {
		t.Fatalf("embedModel() = %q, want empty", got)
	}
}

func TestEdgeChunkText_boundaries(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		text    string
		size    int
		overlap int
		wantLen int
	}{
		{"zero size defaults", "abc", 0, 0, 1},
		{"negative size defaults", "abc", -3, 0, 1},
		{"negative overlap clamped", "abcdefghij", 5, -2, 2},
		{"overlap equals size clamped", "abcdefghij", 4, 4, 4},
		{"overlap larger than size clamped", "abcdefghij", 4, 99, 4},
		{"exact size", "abcde", 5, 0, 1},
		{"size one", "abc", 1, 0, 3},
		{"size one overlap clamped", "abc", 1, 1, 3},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := chunkText(tc.text, tc.size, tc.overlap)
			if len(got) != tc.wantLen {
				t.Fatalf("chunkText() len = %d (%v), want %d", len(got), got, tc.wantLen)
			}
		})
	}
}

func TestEdgeChunkText_multibyte(t *testing.T) {
	t.Parallel()

	text := "héllo世界abc"
	chunks := chunkText(text, 5, 0)
	joined := strings.Join(chunks, "")
	if joined != text {
		t.Fatalf("joined = %q, want %q", joined, text)
	}
	for _, c := range chunks {
		if len([]rune(c)) > 5 {
			t.Fatalf("chunk %q exceeds 5 runes", c)
		}
	}
}

func TestEdgeChunkText_overlapContinuity(t *testing.T) {
	t.Parallel()

	chunks := chunkText("abcdefghij", 5, 2)
	want := []string{"abcde", "defgh", "ghij"}
	if len(chunks) != len(want) {
		t.Fatalf("chunks = %v, want %v", chunks, want)
	}
	for i := range want {
		if chunks[i] != want[i] {
			t.Fatalf("chunks[%d] = %q, want %q", i, chunks[i], want[i])
		}
	}
}

func TestEdgeChunkID(t *testing.T) {
	t.Parallel()

	if got := chunkID("doc", 0); got != "doc#0" {
		t.Fatalf("chunkID() = %q, want doc#0", got)
	}
	if got := chunkID("doc", 12); got != "doc#12" {
		t.Fatalf("chunkID() = %q, want doc#12", got)
	}
}

func TestEdgeNew_chunkSizeZeroAllowed(t *testing.T) {
	t.Parallel()

	e, err := New(&fakeAI{}, &memStore{}, Options{ChunkSize: 0})
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	if e.opts.ChunkSize != DefaultChunkSize {
		t.Fatalf("ChunkSize = %d, want %d", e.opts.ChunkSize, DefaultChunkSize)
	}
}

func TestEdgeIngest_nilAndEmptyDocs(t *testing.T) {
	t.Parallel()

	e := newTestEngine(t, &fakeAI{}, &memStore{}, Options{})

	if err := e.Ingest(t.Context(), nil); err != nil {
		t.Fatalf("Ingest(nil) error = %v, want nil", err)
	}
	if err := e.Ingest(t.Context(), []Document{}); err != nil {
		t.Fatalf("Ingest(empty) error = %v, want nil", err)
	}
}

func TestEdgeIngest_reservedMetadataKeys(t *testing.T) {
	t.Parallel()

	f := &fakeAI{}
	s := &memStore{}
	e := newTestEngine(t, f, s, Options{ChunkSize: 100, ChunkOverlap: 0})

	err := e.Ingest(t.Context(), []Document{{
		ID:      "d1",
		Content: "hello world",
		Metadata: map[string]any{
			"document_id": "spoofed",
			"chunk_index": 99,
			"content":     "spoofed",
			"lang":        "en",
		},
	}})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if len(s.vecs) != 1 {
		t.Fatalf("vecs = %d, want 1", len(s.vecs))
	}
	meta := s.vecs[0].Metadata
	if meta["document_id"] != "d1" || meta["chunk_index"] != 0 || meta["content"] != "hello world" {
		t.Fatalf("reserved keys overwritten: %v", meta)
	}
	if meta["lang"] != "en" {
		t.Fatalf("caller metadata lost: %v", meta)
	}
	if s.vecs[0].ID != "d1#0" {
		t.Fatalf("ID = %q, want d1#0", s.vecs[0].ID)
	}
}

func TestEdgeIngest_multiDocChunkIDs(t *testing.T) {
	t.Parallel()

	s := &memStore{}
	e := newTestEngine(t, &fakeAI{}, s, Options{ChunkSize: 3, ChunkOverlap: 0})

	err := e.Ingest(t.Context(), []Document{
		{ID: "a", Content: "abcdef"},
		{ID: "b", Content: "xyz"},
	})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if len(s.vecs) != 3 {
		t.Fatalf("vecs = %d, want 3", len(s.vecs))
	}
	wantIDs := []string{"a#0", "a#1", "b#0"}
	for i, want := range wantIDs {
		if s.vecs[i].ID != want {
			t.Fatalf("vecs[%d].ID = %q, want %q", i, s.vecs[i].ID, want)
		}
	}
}

func TestEdgeIngest_nilMetadata(t *testing.T) {
	t.Parallel()

	s := &memStore{}
	e := newTestEngine(t, &fakeAI{}, s, Options{ChunkSize: 100})

	if err := e.Ingest(t.Context(), []Document{{ID: "d", Content: "hi"}}); err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if s.vecs[0].Metadata["document_id"] != "d" {
		t.Fatalf("metadata = %v, want document_id d", s.vecs[0].Metadata)
	}
}

func TestEdgeIngest_storeError(t *testing.T) {
	t.Parallel()

	want := errors.New("store down")
	e, err := New(&fakeAI{}, &stubErrStore{err: want}, Options{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := e.Ingest(t.Context(), []Document{{ID: "d", Content: "hello"}}); !errors.Is(err, want) {
		t.Fatalf("Ingest() error = %v, want %v", err, want)
	}
}

func TestEdgeIngest_canceledContext(t *testing.T) {
	t.Parallel()

	e, err := New(&stubCtxAI{}, &stubCtxStore{}, Options{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := e.Ingest(ctx, []Document{{ID: "d", Content: "hello"}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Ingest() error = %v, want context.Canceled", err)
	}
}

func TestEdgeRetrieve_embedCountZero(t *testing.T) {
	t.Parallel()

	e := newTestEngine(t, &fakeAI{embeds: [][]float32{}}, &memStore{}, Options{})

	if _, err := e.Retrieve(t.Context(), "q", 3); !errors.Is(err, ErrEmbeddingCount) {
		t.Fatalf("Retrieve() error = %v, want ErrEmbeddingCount", err)
	}
}

func TestEdgeRetrieve_embedCountTwo(t *testing.T) {
	t.Parallel()

	e := newTestEngine(t, &fakeAI{embeds: [][]float32{{1}, {2}}}, &memStore{}, Options{})

	if _, err := e.Retrieve(t.Context(), "q", 3); !errors.Is(err, ErrEmbeddingCount) {
		t.Fatalf("Retrieve() error = %v, want ErrEmbeddingCount", err)
	}
}

func TestEdgeRetrieve_embedError(t *testing.T) {
	t.Parallel()

	want := errors.New("embed down")
	e := newTestEngine(t, &fakeAI{embedErr: want}, &memStore{}, Options{})

	if _, err := e.Retrieve(t.Context(), "q", 3); !errors.Is(err, want) {
		t.Fatalf("Retrieve() error = %v, want %v", err, want)
	}
}

func TestEdgeRetrieve_storeError(t *testing.T) {
	t.Parallel()

	want := errors.New("query down")
	e, err := New(&fakeAI{}, &stubQueryErrStore{err: want}, Options{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := e.Retrieve(t.Context(), "q", 3); !errors.Is(err, want) {
		t.Fatalf("Retrieve() error = %v, want %v", err, want)
	}
}

func TestEdgeRetrieve_negativeTopKDefaults(t *testing.T) {
	t.Parallel()

	f := &fakeAI{}
	s := &memStore{vecs: []vectorstore.Vector{
		{ID: "a#0", Metadata: map[string]any{"content": "one"}},
		{ID: "a#1", Metadata: map[string]any{"content": "two"}},
	}}
	e := newTestEngine(t, f, s, Options{TopK: 1})

	got, err := e.Retrieve(t.Context(), "q", -5)
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("sources = %d, want 1 (TopK default)", len(got))
	}
}

func TestEdgeRetrieve_missingContentKey(t *testing.T) {
	t.Parallel()

	s := &memStore{vecs: []vectorstore.Vector{
		{ID: "a#0", Metadata: map[string]any{"other": 1}},
	}}
	e := newTestEngine(t, &fakeAI{}, s, Options{})

	got, err := e.Retrieve(t.Context(), "q", 3)
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("sources = %d, want 1", len(got))
	}
	if got[0].Content != "" {
		t.Fatalf("Content = %q, want empty", got[0].Content)
	}
	if got[0].Metadata["other"] != 1 {
		t.Fatalf("Metadata = %v, want passthrough", got[0].Metadata)
	}
}

func TestEdgeRetrieve_canceledContext(t *testing.T) {
	t.Parallel()

	e, err := New(&stubCtxAI{}, &stubCtxStore{}, Options{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := e.Retrieve(ctx, "q", 3); !errors.Is(err, context.Canceled) {
		t.Fatalf("Retrieve() error = %v, want context.Canceled", err)
	}
}

func TestEdgeAnswer_generateError(t *testing.T) {
	t.Parallel()

	want := errors.New("generate down")
	f := &fakeAI{genErr: want}
	s := &memStore{vecs: []vectorstore.Vector{
		{ID: "a#0", Metadata: map[string]any{"content": "ctx"}},
	}}
	e := newTestEngine(t, f, s, Options{})

	if _, err := e.Answer(t.Context(), "q?", 1); !errors.Is(err, want) {
		t.Fatalf("Answer() error = %v, want %v", err, want)
	}
}

func TestEdgeAnswer_retrieveError(t *testing.T) {
	t.Parallel()

	e := newTestEngine(t, &fakeAI{}, &memStore{}, Options{})

	if _, err := e.Answer(t.Context(), "", 3); !errors.Is(err, ErrEmptyQuery) {
		t.Fatalf("Answer() error = %v, want ErrEmptyQuery", err)
	}
}

func TestEdgeAnswer_emptySourcesStillGenerates(t *testing.T) {
	t.Parallel()

	f := &fakeAI{gen: ai.Generation{Content: "no context"}}
	e := newTestEngine(t, f, &memStore{}, Options{})

	ans, err := e.Answer(t.Context(), "q?", 3)
	if err != nil {
		t.Fatalf("Answer() error = %v", err)
	}
	if ans.Text != "no context" {
		t.Fatalf("Text = %q, want no context", ans.Text)
	}
	if len(ans.Sources) != 0 {
		t.Fatalf("Sources = %d, want 0", len(ans.Sources))
	}
}

func TestEdgeAnswer_customSystemPrompt(t *testing.T) {
	t.Parallel()

	f := &fakeAI{gen: ai.Generation{Content: "x"}}
	s := &memStore{vecs: []vectorstore.Vector{
		{ID: "a#0", Metadata: map[string]any{"content": "ctx"}},
	}}
	e := newTestEngine(t, f, s, Options{SystemPrompt: "custom prompt"})

	if _, err := e.Answer(t.Context(), "q?", 1); err != nil {
		t.Fatalf("Answer() error = %v", err)
	}
	if f.lastMsgs[0].Content != "custom prompt" {
		t.Fatalf("system = %q, want custom prompt", f.lastMsgs[0].Content)
	}
}

func TestEdgeAnswer_defaultSystemPromptAndCitations(t *testing.T) {
	t.Parallel()

	f := &fakeAI{gen: ai.Generation{Content: "x"}}
	s := &memStore{vecs: []vectorstore.Vector{
		{ID: "a#0", Metadata: map[string]any{"content": "first"}},
		{ID: "a#1", Metadata: map[string]any{"content": "second"}},
	}}
	e := newTestEngine(t, f, s, Options{})

	if _, err := e.Answer(t.Context(), "q?", 2); err != nil {
		t.Fatalf("Answer() error = %v", err)
	}
	if f.lastMsgs[0].Role != ai.RoleSystem || f.lastMsgs[0].Content == "" {
		t.Fatalf("system message = %+v, want non-empty", f.lastMsgs[0])
	}

	var user string
	for _, m := range f.lastMsgs {
		if m.Role == ai.RoleUser {
			user = m.Content
		}
	}
	if !strings.Contains(user, "[1] first") || !strings.Contains(user, "[2] second") {
		t.Fatalf("user prompt = %q, want [1]/[2] citations", user)
	}
	if !strings.Contains(user, "Question: q?") {
		t.Fatalf("user prompt = %q, want question", user)
	}
}

// stubSafeAI is a goroutine-safe ai.AI for concurrency tests.
type stubSafeAI struct{}

func (stubSafeAI) Generate(context.Context, string, []ai.Message, ai.GenerateOptions) (ai.Generation, error) {
	return ai.Generation{Content: "x"}, nil
}

func (stubSafeAI) Stream(context.Context, string, []ai.Message, ai.GenerateOptions) (<-chan ai.StreamChunk, error) {
	ch := make(chan ai.StreamChunk)
	close(ch)
	return ch, nil
}

func (stubSafeAI) Embed(_ context.Context, _ string, inputs []string, _ ai.EmbedOptions) ([][]float32, error) {
	out := make([][]float32, len(inputs))
	for i := range inputs {
		out[i] = []float32{1, 0}
	}
	return out, nil
}

func (stubSafeAI) Close() error { return nil }

// stubSafeStore is a goroutine-safe read-only store for concurrency tests.
type stubSafeStore struct {
	vecs []vectorstore.Vector
	mu   sync.RWMutex
}

func (s *stubSafeStore) Upsert(_ context.Context, v vectorstore.Vector) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vecs = append(s.vecs, v)
	return nil
}

func (s *stubSafeStore) UpsertBatch(_ context.Context, vs []vectorstore.Vector) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vecs = append(s.vecs, vs...)
	return nil
}

func (s *stubSafeStore) Delete(context.Context, string) error { return nil }

func (s *stubSafeStore) Query(_ context.Context, _ []float32, topK int) ([]vectorstore.ScoreMatch, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]vectorstore.ScoreMatch, 0, len(s.vecs))
	for i, v := range s.vecs {
		out = append(out, vectorstore.ScoreMatch{ID: v.ID, Score: float32(len(s.vecs) - i), Metadata: v.Metadata})
	}
	if topK > 0 && len(out) > topK {
		out = out[:topK]
	}
	return out, nil
}

func (s *stubSafeStore) Close() error { return nil }

func TestEdgeEngine_concurrent(t *testing.T) {
	t.Parallel()

	s := &stubSafeStore{vecs: []vectorstore.Vector{
		{ID: "a#0", Metadata: map[string]any{"content": "ctx"}},
	}}
	e, err := New(stubSafeAI{}, s, Options{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = e.Retrieve(context.Background(), "q", 1)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("Retrieve(%d) error = %v", i, err)
		}
	}
}

func TestEdgeErrorTypes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want error
	}{
		{"invalid options", InvalidOptionsError{Reason: "r"}, ErrInvalidOptions},
		{"invalid document", InvalidDocumentError{Reason: "r"}, ErrInvalidDocument},
		{"embedding count", EmbeddingCountError{Got: 1, Want: 2}, ErrEmbeddingCount},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if !errors.Is(tc.err, tc.want) {
				t.Fatalf("%T error = %v, want %v", tc.err, tc.err, tc.want)
			}
			if !strings.HasPrefix(tc.err.Error(), strings.Split(tc.want.Error(), ":")[0]+":") {
				t.Fatalf("Error() = %q, want package prefix", tc.err.Error())
			}
		})
	}
}

func TestEdgeEmbeddingCountError_format(t *testing.T) {
	t.Parallel()

	err := EmbeddingCountError{Got: 1, Want: 3}
	if !strings.Contains(err.Error(), "1") || !strings.Contains(err.Error(), "3") {
		t.Fatalf("Error() = %q, want got/want counts", err.Error())
	}
}
