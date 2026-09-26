package adapters

import (
	"github.com/zenta-dev/zever/search"
	searchmeilisearch "github.com/zenta-dev/zever/search/meilisearch"
	"github.com/zenta-dev/zever/vectorstore"
	vectorstoreqdrant "github.com/zenta-dev/zever/vectorstore/qdrant"
)

// RegisterSearchVector registers the external search and vector adapters
// the core container no longer imports: search/meilisearch and
// vectorstore/qdrant. The postgres/sqlite adapters on both facades stay
// wired by the container. Registration only fills factory maps; it performs
// no I/O. Duplicate registrations are ignored, so calling
// RegisterSearchVector more than once (or alongside RegisterAll) is safe.
func RegisterSearchVector() {
	_ = search.Register(search.Meilisearch, searchmeilisearch.New)

	_ = vectorstore.Register(vectorstore.Qdrant, vectorstoreqdrant.New)
}
