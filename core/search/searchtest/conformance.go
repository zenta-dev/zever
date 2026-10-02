// Package searchtest provides the conformance kit third-party search adapters run to prove backend parity.
package searchtest

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/search"
)

// Conformance verifies factory-built search backends implement the
// search.Search contract: Index/IndexBatch round-trip through Search with an
// explicit index filter, overwrite on re-index, Delete with NotFound sentinels,
// invalid-metadata rejection, default-limit behavior, offset paging, and
// Close. Each subtest takes a fresh instance from factory and uses its own
// index name so cases stay isolated even on shared live servers. Tests are
// deterministic and touch no network.
func Conformance(t *testing.T, factory func(t *testing.T) search.Search) {
	t.Helper()

	t.Run("IndexSearch", func(t *testing.T) { conformanceIndexSearch(t, factory) })
	t.Run("IndexBatch", func(t *testing.T) { conformanceIndexBatch(t, factory) })
	t.Run("Overwrite", func(t *testing.T) { conformanceOverwrite(t, factory) })
	t.Run("DeleteMissing", func(t *testing.T) { conformanceDeleteMissing(t, factory) })
	t.Run("DeleteRoundTrip", func(t *testing.T) { conformanceDeleteRoundTrip(t, factory) })
	t.Run("InvalidMetadata", func(t *testing.T) { conformanceInvalidMetadata(t, factory) })
	t.Run("LimitDefault", func(t *testing.T) { conformanceLimitDefault(t, factory) })
	t.Run("OffsetBeyond", func(t *testing.T) { conformanceOffsetBeyond(t, factory) })
	t.Run("Close", func(t *testing.T) { conformanceClose(t, factory) })
}

// mustIndex fails the test on Index error.
func mustIndex(t *testing.T, s search.Search, doc search.Document) {
	t.Helper()

	if err := s.Index(t.Context(), doc); err != nil {
		t.Fatalf("Index(%q) error = %v", doc.ID, err)
	}
}

// mustSearchIndex runs query filtered to idx and fails the test on error.
func mustSearchIndex(t *testing.T, s search.Search, query, idx string, limit int) search.Result {
	t.Helper()

	res, err := s.Search(t.Context(), query, search.QueryOptions{
		Limit:   limit,
		Filters: map[string]string{"index": idx},
	})
	if err != nil {
		t.Fatalf("Search(%q) error = %v", query, err)
	}

	return res
}

// hitIDs returns the hit ID set for membership assertions.
func hitIDs(res search.Result) map[string]bool {
	ids := make(map[string]bool, len(res.Hits))
	for _, h := range res.Hits {
		ids[h.ID] = true
	}

	return ids
}

func conformanceIndexSearch(t *testing.T, factory func(t *testing.T) search.Search) {
	t.Helper()

	const idx = "kit-idx-search"
	s := factory(t)

	mustIndex(t, s, search.Document{ID: "kit-s-alpha", Index: idx, Content: "conformancealpha bright meadow"})
	mustIndex(t, s, search.Document{ID: "kit-s-beta", Index: idx, Content: "conformancebeta silent harbor"})

	res := mustSearchIndex(t, s, "conformancealpha", idx, 10)

	if res.Total < 1 || len(res.Hits) < 1 {
		t.Fatalf("Search() = %+v, want at least one hit", res)
	}

	if !hitIDs(res)["kit-s-alpha"] {
		t.Errorf("Search() hits = %v, want kit-s-alpha", res.Hits)
	}
}

func conformanceIndexBatch(t *testing.T, factory func(t *testing.T) search.Search) {
	t.Helper()

	const idx = "kit-idx-batch"
	s := factory(t)

	docs := []search.Document{
		{ID: "kit-b-one", Index: idx, Content: "conformancebatch first pine forest"},
		{ID: "kit-b-two", Index: idx, Content: "conformancebatch second pine forest"},
	}
	if err := s.IndexBatch(t.Context(), docs); err != nil {
		t.Fatalf("IndexBatch() error = %v", err)
	}

	res := mustSearchIndex(t, s, "conformancebatch", idx, 10)

	if res.Total != 2 {
		t.Errorf("Search().Total = %d, want 2", res.Total)
	}

	ids := hitIDs(res)
	if !ids["kit-b-one"] || !ids["kit-b-two"] {
		t.Errorf("Search() hits = %v, want kit-b-one and kit-b-two", res.Hits)
	}
}

func conformanceOverwrite(t *testing.T, factory func(t *testing.T) search.Search) {
	t.Helper()

	const idx = "kit-idx-overwrite"
	s := factory(t)
	ctx := t.Context()

	mustIndex(t, s, search.Document{ID: "kit-o-doc", Index: idx, Content: "conformanceoverwrite original juniper"})

	if err := s.Index(ctx, search.Document{ID: "kit-o-doc", Index: idx, Content: "conformanceoverwrite replaced sequoia"}); err != nil {
		t.Fatalf("Index(overwrite) error = %v", err)
	}

	res := mustSearchIndex(t, s, "conformanceoverwrite", idx, 10)

	if res.Total != 1 {
		t.Errorf("Search().Total = %d, want 1 after overwrite", res.Total)
	}

	if !hitIDs(res)["kit-o-doc"] {
		t.Errorf("Search() hits = %v, want kit-o-doc", res.Hits)
	}
}

func conformanceDeleteMissing(t *testing.T, factory func(t *testing.T) search.Search) {
	t.Helper()

	err := factory(t).Delete(t.Context(), "kit-no-such-doc")

	var nfErr *search.NotFoundError
	if !errors.As(err, &nfErr) {
		t.Fatalf("Delete(missing) err = %T %v, want *NotFoundError", err, err)
	}

	if nfErr.ID != "kit-no-such-doc" {
		t.Errorf("NotFoundError.ID = %q, want kit-no-such-doc", nfErr.ID)
	}

	if !errors.Is(err, search.ErrNotFound) {
		t.Errorf("errors.Is(err, ErrNotFound) = false (err = %v)", err)
	}
}

func conformanceDeleteRoundTrip(t *testing.T, factory func(t *testing.T) search.Search) {
	t.Helper()

	const idx = "kit-idx-delete"
	s := factory(t)
	ctx := t.Context()

	mustIndex(t, s, search.Document{ID: "kit-d-doc", Index: idx, Content: "conformancedelete fading ember"})

	if res := mustSearchIndex(t, s, "conformancedelete", idx, 10); res.Total != 1 {
		t.Fatalf("Search().Total = %d, want 1 before delete", res.Total)
	}

	if err := s.Delete(ctx, "kit-d-doc"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if res := mustSearchIndex(t, s, "conformancedelete", idx, 10); res.Total != 0 || len(res.Hits) != 0 {
		t.Errorf("Search() = %+v, want empty after delete", res)
	}

	if err := s.Delete(ctx, "kit-d-doc"); !errors.Is(err, search.ErrNotFound) {
		t.Errorf("Delete(again) err = %v, want ErrNotFound", err)
	}
}

func conformanceInvalidMetadata(t *testing.T, factory func(t *testing.T) search.Search) {
	t.Helper()

	s := factory(t)

	bad := search.Document{
		ID:       "kit-bad-meta",
		Index:    "kit-idx-meta",
		Content:  "metadata probe",
		Metadata: map[string]any{"ch": make(chan int)},
	}

	if err := s.Index(t.Context(), bad); !errors.Is(err, search.ErrInvalidMetadata) {
		t.Errorf("Index(bad metadata) err = %v, want ErrInvalidMetadata", err)
	}

	if err := s.IndexBatch(t.Context(), []search.Document{bad}); !errors.Is(err, search.ErrInvalidMetadata) {
		t.Errorf("IndexBatch(bad metadata) err = %v, want ErrInvalidMetadata", err)
	}
}

func conformanceLimitDefault(t *testing.T, factory func(t *testing.T) search.Search) {
	t.Helper()

	const idx = "kit-idx-limit"
	s := factory(t)

	mustIndex(t, s, search.Document{ID: "kit-l-doc", Index: idx, Content: "conformancelimit quiet orchard"})

	res := mustSearchIndex(t, s, "conformancelimit", idx, 0)

	if res.Total < 1 || len(res.Hits) < 1 {
		t.Errorf("Search(limit 0) = %+v, want default-limit hit", res)
	}

	if len(res.Hits) > search.DefaultLimit {
		t.Errorf("Search(limit 0) hits = %d, want <= DefaultLimit %d", len(res.Hits), search.DefaultLimit)
	}
}

func conformanceOffsetBeyond(t *testing.T, factory func(t *testing.T) search.Search) {
	t.Helper()

	const idx = "kit-idx-offset"
	s := factory(t)
	ctx := t.Context()

	mustIndex(t, s, search.Document{ID: "kit-f-doc", Index: idx, Content: "conformanceoffset lonely lighthouse"})

	res, err := s.Search(ctx, "conformanceoffset", search.QueryOptions{
		Limit:   10,
		Offset:  100,
		Filters: map[string]string{"index": idx},
	})
	if err != nil {
		t.Fatalf("Search(offset) error = %v", err)
	}

	if res.Total < 1 {
		t.Errorf("Search(offset).Total = %d, want >= 1", res.Total)
	}

	if len(res.Hits) != 0 {
		t.Errorf("Search(offset) hits = %v, want none beyond total", res.Hits)
	}
}

func conformanceClose(t *testing.T, factory func(t *testing.T) search.Search) {
	t.Helper()

	s := factory(t)

	if err := s.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := s.Close(); err != nil {
		t.Errorf("Close() second error = %v, want nil", err)
	}
}
