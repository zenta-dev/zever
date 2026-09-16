package orm

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/db"
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
// placeholder limits (Postgres ~65535 params, SQLite ~999-32766 depending on
// build flags) or a degenerate query plan. 1000 is
// conservative across both dialects at once: Preload has no per-call
// dialect context to branch on, so it picks one constant safe everywhere
// rather than guessing the backend.
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

	var children []PC

	// The id set is fanned out into inChunkSize slices so no single child
	// query carries an unbounded placeholder list.
	for start := 0; start < len(ids); start += inChunkSize {
		end := start + inChunkSize
		if end > len(ids) {
			end = len(ids)
		}

		chunkRows, err := childBase.Where(childFK.In(ids[start:end]...)).All(ctx, exec)
		if err != nil {
			return nil, fmt.Errorf("orm: Preload: children: %w", err)
		}

		children = append(children, chunkRows...)
	}

	grouped := make(map[K][]*C, len(rows))

	for _, child := range children {
		key := childFKOf((*C)(child))
		grouped[key] = append(grouped[key], (*C)(child))
	}

	out := make([]ParentWithChildren[T, C], len(rows))
	for i, row := range rows {
		parent := (*T)(row)
		out[i] = ParentWithChildren[T, C]{Parent: parent, Children: grouped[parentID(parent)]}
	}

	return out, nil
}
