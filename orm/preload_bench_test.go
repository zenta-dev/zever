package orm

import (
	"testing"
)

// BenchmarkPreloadBelowChunkSize measures Preload cost for a parent count
// well below inChunkSize (so the child query loop always executes exactly
// one iteration, same as the pre-chunking direct call) -- this is the
// no-regression comparison point: chunking a below-threshold id set should
// cost the same as the old unconditional single query, modulo negligible
// loop overhead around the one iteration that actually runs.
func BenchmarkPreloadBelowChunkSize(b *testing.B) {
	ctx, conn := newPreloadDBWithParents(b, 200)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		got, err := Preload(
			ctx, conn,
			From(preloadAuthors).OrderBy(preloadAuthorID.Asc()),
			preloadBookAuthor,
			From(preloadBooks),
			preloadAuthorKey,
			preloadBookFK,
		)
		if err != nil {
			b.Fatal(err)
		}

		_ = len(got)
	}
}

// BenchmarkPreloadAboveChunkSize measures Preload cost for a parent count
// several multiples of inChunkSize, so the child IN query is chunked into
// multiple round trips -- the comparison point showing the actual cost of
// chunking when it does kick in.
func BenchmarkPreloadAboveChunkSize(b *testing.B) {
	ctx, conn := newPreloadDBWithParents(b, inChunkSize*5)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		got, err := Preload(
			ctx, conn,
			From(preloadAuthors).OrderBy(preloadAuthorID.Asc()),
			preloadBookAuthor,
			From(preloadBooks),
			preloadAuthorKey,
			preloadBookFK,
		)
		if err != nil {
			b.Fatal(err)
		}

		_ = len(got)
	}
}
