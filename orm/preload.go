package orm

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/orm/dialect"
)

// ParentWithChildren is the result of Preload: one parent T with all of its
// children C attached, in the order the caller's child query requested.
// Children is nil (not a zero-length non-nil slice) when the parent has no
// children -- a parent with zero children is still present in the result,
// just with no attached children.
type ParentWithChildren[T any, C any] struct {
	Parent   *T
	Children []*C
}

// inChunkSize caps how many parent ids go into a single generated `childFK
// IN (...)` query. Without a cap, a very large parent set would produce one
// SQL statement with as many bound placeholders as ids, risking driver/DB
// placeholder limits (Postgres 65535 params, SQLite 999 before 3.32.0 and
// 32766 after) or a degenerate query plan. 1000 is conservative across
// both dialects at once -- and Preload caps it further per call against
// the resolved dialect's own budget (minus the child query's other bound
// arguments), so an old-SQLite backend with a 999 budget still gets
// fitting statements instead of a driver error.
const inChunkSize = 1000

// Preload loads every parent matched by parents and attaches its children in
// exactly TWO ROUND TRIPS worth of query SHAPES, regardless of how many
// parents or children there are -- the N+1-free guarantee:
//
// - Query 1: parents.All, materializing the parent rows.
// - Query 2: childBase with `childFK IN (<the fetched parent ids>)` ANDed
// in, materializing every child of any fetched parent. When the parent
// id set is larger than inChunkSize, this becomes multiple queries --
// one per chunk of ids -- instead of a single query with an unbounded
// number of placeholders; their results are concatenated before
// grouping, so callers with small parent sets still see exactly one
// child query.
//
// The children of each parent are grouped by key with zero reflection: the
// caller supplies parentID (a parent's key) and childFKOf (a child's foreign
// key), so the helper never inspects struct fields. childFK is the typed
// child-side foreign-key Column, used to build the IN predicate through the
// normal Column.In path -- so parent ids are bound as placeholders, never
// interpolated into SQL text.
//
// Child-side filtering and ordering come from childBase itself: chain
// childBase.Where(...)/.OrderBy(...) before calling Preload, and the filter
// is ANDed with the generated FK IN while the ordering is preserved per
// parent. When parents matches no rows, the child query is skipped entirely
// and the result is empty.
//
// See the package doc comment for how a genuine one-to-many Preload differs
// from the single-composite-row Join2/Join3 builders.
func Preload[T any, PT ptrScanner[T], C any, PC ptrScanner[C], K comparable](
	ctx context.Context,
	exec db.DB,
	parents Query[T, PT],
	childFK Column[C, K],
	childBase Query[C, PC],
	parentID func(*T) K,
	childFKOf func(*C) K,
) ([]ParentWithChildren[T, C], error) {
	rows, err := parents.All(ctx, exec)
	if err != nil {
		return nil, fmt.Errorf("orm: Preload: parents: %w", err)
	}

	if len(rows) == 0 {
		return nil, nil
	}

	ids := make([]K, len(rows))
	for i, row := range rows {
		ids[i] = parentID((*T)(row))
	}

	// Size the child-IN chunks against the resolved dialect's parameter
	// budget (minus the child query's own non-IN arguments), so the
	// generated statements always fit: the 1000 default stands on modern
	// backends, while an old-SQLite 999 budget transparently shrinks the
	// chunks instead of failing in the driver.
	chunkSize, err := preloadChunkSize(childBase, exec)
	if err != nil {
		return nil, err
	}

	// Each chunk's rows are kept as their own slice rather than concatenated
	// into one growing slice: grouping below walks them twice, and retaining
	// the per-chunk slices avoids the repeated reallocation that appending
	// every chunk into a single slice would pay as chunks accumulate.
	chunks := make([][]PC, 0, (len(ids)+chunkSize-1)/chunkSize)

	// The id set is fanned out into chunkSize slices so no single child
	// query carries an unbounded placeholder list.
	for start := 0; start < len(ids); start += chunkSize {
		end := start + chunkSize
		if end > len(ids) {
			end = len(ids)
		}

		chunkRows, err := childBase.Where(childFK.In(ids[start:end]...)).All(ctx, exec)
		if err != nil {
			return nil, fmt.Errorf("orm: Preload: children: %w", err)
		}

		chunks = append(chunks, chunkRows)
	}

	// groupSpan locates one key's children inside all: count is fixed by the
	// counting pass, start advances as the filling pass runs, so every
	// parent's children slice aliases all instead of each key allocating its
	// own one-element backing array.
	type groupSpan struct {
		start int
		count int
	}

	grouped := make(map[K]groupSpan, len(rows))

	var total int

	for _, chunk := range chunks {
		for _, child := range chunk {
			key := childFKOf((*C)(child))

			span := grouped[key]
			span.count++
			grouped[key] = span

			total++
		}
	}

	all := make([]*C, total)

	offset := 0
	for key, span := range grouped {
		span.start = offset
		grouped[key] = span

		offset += span.count
	}

	for _, chunk := range chunks {
		for _, child := range chunk {
			key := childFKOf((*C)(child))

			span := grouped[key]
			all[span.start] = (*C)(child)
			span.start++
			grouped[key] = span
		}
	}

	out := make([]ParentWithChildren[T, C], len(rows))
	for i, row := range rows {
		parent := (*T)(row)

		span := grouped[parentID(parent)]

		var children []*C
		if span.count > 0 {
			// Three-index slice caps the length so a caller that appends to
			// one parent's children can never overwrite the next parent's.
			children = all[span.start-span.count : span.start : span.start]
		}

		out[i] = ParentWithChildren[T, C]{Parent: parent, Children: children}
	}

	return out, nil
}

// preloadChunkSize returns the child-IN chunk size for one Preload call:
// inChunkSize capped by the resolved dialect's parameter budget minus the
// child query's own non-IN bound arguments (its filter/order/limit tail),
// so every generated child statement fits. A budget the child's own
// arguments already fill fails closed with ErrInListTooLarge.
func preloadChunkSize[C any, PC ptrScanner[C]](childBase Query[C, PC], exec db.DB) (int, error) {
	d, err := resolveDialect(exec)
	if err != nil {
		return 0, err
	}

	reserved := countNodeArgs(childBase.where.n)
	if childBase.limit > 0 {
		reserved++
	}

	if childBase.offset > 0 {
		reserved++
	}

	chunkSize := inChunkSize

	if room := dialect.MaxParams(d) - reserved; room < chunkSize {
		chunkSize = room
	}

	if chunkSize < 1 {
		return 0, fmt.Errorf("orm: Preload: %w", ErrInListTooLarge)
	}

	return chunkSize, nil
}
