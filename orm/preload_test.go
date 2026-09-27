package orm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/db"
)

// preloadAuthor/preloadBook are a small has_many fixture pair mirroring what
// schema codegen would generate for a one-to-many relation: two
// plain entity structs, one orm.Table/orm.Column set per entity, and
// pointer-receiver Scan methods reading columns positionally.
type preloadAuthor struct {
	ID   string
	Name string
}

func (a *preloadAuthor) Scan(row Row) error {
	return row.Scan(&a.ID, &a.Name)
}

type preloadBook struct {
	ID       string
	AuthorID string
	Title    string
}

func (b *preloadBook) Scan(row Row) error {
	return row.Scan(&b.ID, &b.AuthorID, &b.Title)
}

var (
	preloadAuthors    = NewTable[preloadAuthor]("preload_authors", []string{"id", "name"})
	preloadAuthorID   = NewColumn[preloadAuthor, string]("preload_authors", "id")
	preloadAuthorName = NewColumn[preloadAuthor, string]("preload_authors", "name")

	preloadBooks      = NewTable[preloadBook]("preload_books", []string{"id", "author_id", "title"})
	preloadBookAuthor = NewColumn[preloadBook, string]("preload_books", "author_id")
	preloadBookTitle  = NewColumn[preloadBook, string]("preload_books", "title")
)

// preloadAuthorKey and preloadBookFK are the zero-reflection accessors the
// preload helper needs: the parent's primary key and the child's foreign key.
func preloadAuthorKey(a *preloadAuthor) string { return a.ID }
func preloadBookFK(b *preloadBook) string      { return b.AuthorID }

const preloadParentCount = 100

// newPreloadDB opens an in-memory sqlite database seeded with 100 authors;
// author i owns exactly (i % 5) books, so the fixture covers parents with
// zero children through parents with four, and the many-children case.
func newPreloadDB(t *testing.T) (context.Context, db.DB) {
	t.Helper()

	ctx := context.Background()

	conn := openORMTestDB(ctx, t)

	if _, err := conn.Exec(ctx, `CREATE TABLE preload_authors (id text, name text)`); err != nil {
		t.Fatalf("create preload_authors: %v", err)
	}

	if _, err := conn.Exec(ctx, `CREATE TABLE preload_books (id text, author_id text, title text)`); err != nil {
		t.Fatalf("create preload_books: %v", err)
	}

	for i := range preloadParentCount {
		id := fmt.Sprintf("a%03d", i)
		name := fmt.Sprintf("author-%03d", i)

		if _, err := conn.Exec(ctx, `INSERT INTO preload_authors (id, name) VALUES (?, ?)`, id, name); err != nil {
			t.Fatalf("insert author: %v", err)
		}

		// Insert titles in DESCENDING order so a child-side ascending
		// ORDER BY has to reorder them: any failure to apply/preserve the
		// child order would leave the inserted (descending) sequence.
		for j := range i % 5 {
			bid := fmt.Sprintf("b%03d_%d", i, j)
			title := fmt.Sprintf("book-%03d-%d", i, 4-j)

			if _, err := conn.Exec(ctx, `INSERT INTO preload_books (id, author_id, title) VALUES (?, ?, ?)`, bid, id, title); err != nil {
				t.Fatalf("insert book: %v", err)
			}
		}
	}

	return ctx, conn
}

// preloadSpy is a db.DB spy that records the SQL text of every Query call,
// so a test can both count round trips (the N+1-free guarantee) and assert
// the IN list is placeholder-bound rather than interpolated.
type preloadSpy struct {
	db.DB

	queries []string
}

func (r *preloadSpy) Query(ctx context.Context, query string, args ...any) (db.Rows, error) {
	r.queries = append(r.queries, query)

	return r.DB.Query(ctx, query, args...) //nolint:wrapcheck // test double, error passes straight through
}

// TestPreloadIsTwoQueriesForHundredParents is THE N+1-free proof: loading 100
// parents with all their children issues exactly two queries -- one for the
// parents, one for every child whose FK is IN the parent id set -- never one
// query per parent.
func TestPreloadIsTwoQueriesForHundredParents(t *testing.T) {
	ctx, conn := newPreloadDB(t)

	spy := &preloadSpy{DB: conn}

	got, err := Preload(
		ctx, spy,
		From(preloadAuthors).OrderBy(preloadAuthorID.Asc()),
		preloadBookAuthor,
		From(preloadBooks),
		preloadAuthorKey,
		preloadBookFK,
	)
	if err != nil {
		t.Fatalf("Preload: %v", err)
	}

	if len(got) != preloadParentCount {
		t.Fatalf("len(got) = %d, want %d", len(got), preloadParentCount)
	}

	if len(spy.queries) != 2 {
		t.Fatalf("Preload issued %d queries, want exactly 2:\n%s", len(spy.queries), strings.Join(spy.queries, "\n"))
	}

	if !strings.Contains(spy.queries[1], "IN (") || !strings.Contains(spy.queries[1], "?") {
		t.Fatalf("child query = %q, want a placeholder-bound IN (...)", spy.queries[1])
	}

	if strings.Contains(spy.queries[1], "a001") {
		t.Fatalf("child query = %q, want parent ids bound as placeholders, never interpolated", spy.queries[1])
	}
}

// TestPreloadGroupsChildren proves a real-SQLite round trip attaches each
// child under the right parent, preserves the caller's child ordering, and
// keeps parents that have zero children.
func TestPreloadGroupsChildren(t *testing.T) {
	ctx, conn := newPreloadDB(t)

	got, err := Preload(
		ctx, conn,
		From(preloadAuthors).OrderBy(preloadAuthorID.Asc()),
		preloadBookAuthor,
		From(preloadBooks).OrderBy(preloadBookTitle.Asc()),
		preloadAuthorKey,
		preloadBookFK,
	)
	if err != nil {
		t.Fatalf("Preload: %v", err)
	}

	if len(got) != preloadParentCount {
		t.Fatalf("len(got) = %d, want %d", len(got), preloadParentCount)
	}

	zeroChildParents := 0

	for i, pc := range got {
		wantID := fmt.Sprintf("a%03d", i)

		if pc.Parent == nil || pc.Parent.ID != wantID {
			t.Fatalf("got[%d].Parent = %+v, want ID %q (parent order must be preserved)", i, pc.Parent, wantID)
		}

		wantChildren := i % 5
		if len(pc.Children) != wantChildren {
			t.Fatalf("author %s has %d children, want %d", wantID, len(pc.Children), wantChildren)
		}

		if wantChildren == 0 {
			zeroChildParents++

			continue
		}

		for j, child := range pc.Children {
			if child.AuthorID != wantID {
				t.Fatalf("author %s child %d belongs to %q", wantID, j, child.AuthorID)
			}

			if j > 0 && pc.Children[j-1].Title > child.Title {
				t.Fatalf("author %s children out of order at %d: %q then %q", wantID, j, pc.Children[j-1].Title, child.Title)
			}
		}
	}

	if zeroChildParents == 0 {
		t.Fatalf("no zero-child parents in result; want parents with no children preserved")
	}
}

// TestPreloadChildFilter proves the caller-supplied child query's WHERE is
// ANDed with the generated FK IN filter: only matching children attach, and
// parents whose children were all filtered out survive with none.
func TestPreloadChildFilter(t *testing.T) {
	ctx, conn := newPreloadDB(t)

	got, err := Preload(
		ctx, conn,
		From(preloadAuthors).OrderBy(preloadAuthorID.Asc()),
		preloadBookAuthor,
		From(preloadBooks).Where(preloadBookTitle.Eq("book-004-4")),
		preloadAuthorKey,
		preloadBookFK,
	)
	if err != nil {
		t.Fatalf("Preload: %v", err)
	}

	var total int

	for _, pc := range got {
		for _, child := range pc.Children {
			if child.Title != "book-004-4" {
				t.Fatalf("author %s got child %q, want only book-004-4", pc.Parent.ID, child.Title)
			}

			total++
		}
	}

	if total != 1 {
		t.Fatalf("total children = %d, want 1 (only author a004 owns book-004-4)", total)
	}

	if len(got) != preloadParentCount {
		t.Fatalf("len(got) = %d, want %d (parents survive filtering)", len(got), preloadParentCount)
	}
}

// newPreloadDBWithParents opens an in-memory sqlite database seeded with n
// authors, each owning exactly one book, for tests that need to control the
// exact parent count (e.g. crossing the inChunkSize boundary) rather than
// the fixed newPreloadDB fixture.
func newPreloadDBWithParents(t ormTestCleaner, n int) (context.Context, db.DB) {
	t.Helper()

	ctx := context.Background()

	conn := openORMTestDB(ctx, t)

	if _, err := conn.Exec(ctx, `CREATE TABLE preload_authors (id text, name text)`); err != nil {
		t.Fatalf("create preload_authors: %v", err)
	}

	if _, err := conn.Exec(ctx, `CREATE TABLE preload_books (id text, author_id text, title text)`); err != nil {
		t.Fatalf("create preload_books: %v", err)
	}

	for i := range n {
		id := fmt.Sprintf("a%05d", i)
		name := fmt.Sprintf("author-%05d", i)

		if _, err := conn.Exec(ctx, `INSERT INTO preload_authors (id, name) VALUES (?, ?)`, id, name); err != nil {
			t.Fatalf("insert author: %v", err)
		}

		bid := fmt.Sprintf("b%05d", i)
		title := fmt.Sprintf("book-%05d", i)

		if _, err := conn.Exec(ctx, `INSERT INTO preload_books (id, author_id, title) VALUES (?, ?, ?)`, bid, id, title); err != nil {
			t.Fatalf("insert book: %v", err)
		}
	}

	return ctx, conn
}

// TestPreloadChunksLargeInLists proves the child IN query is chunked at
// inChunkSize: it drives parent counts straddling the boundary
// (inChunkSize-1, inChunkSize, inChunkSize+1) and asserts both the number of
// underlying child queries equals ceil(n/inChunkSize) and every child is
// attached to the right parent with no duplicates and none missing.
func TestPreloadChunksLargeInLists(t *testing.T) {
	for _, n := range []int{inChunkSize - 1, inChunkSize, inChunkSize + 1} {
		t.Run(fmt.Sprintf("n=%d", n), func(t *testing.T) {
			ctx, conn := newPreloadDBWithParents(t, n)

			spy := &preloadSpy{DB: conn}

			got, err := Preload(
				ctx, spy,
				From(preloadAuthors).OrderBy(preloadAuthorID.Asc()),
				preloadBookAuthor,
				From(preloadBooks),
				preloadAuthorKey,
				preloadBookFK,
			)
			if err != nil {
				t.Fatalf("Preload: %v", err)
			}

			if len(got) != n {
				t.Fatalf("len(got) = %d, want %d", len(got), n)
			}

			wantChildQueries := (n + inChunkSize - 1) / inChunkSize
			gotChildQueries := len(spy.queries) - 1 // minus the one parent query

			if gotChildQueries != wantChildQueries {
				t.Fatalf("Preload issued %d child queries, want %d (ceil(%d/%d))",
					gotChildQueries, wantChildQueries, n, inChunkSize)
			}

			seen := make(map[string]bool, n)

			for i, pc := range got {
				wantID := fmt.Sprintf("a%05d", i)

				if pc.Parent == nil || pc.Parent.ID != wantID {
					t.Fatalf("got[%d].Parent = %+v, want ID %q", i, pc.Parent, wantID)
				}

				if len(pc.Children) != 1 {
					t.Fatalf("author %s has %d children, want exactly 1", wantID, len(pc.Children))
				}

				child := pc.Children[0]
				if child.AuthorID != wantID {
					t.Fatalf("author %s got child belonging to %q", wantID, child.AuthorID)
				}

				if seen[child.ID] {
					t.Fatalf("child %q attached more than once", child.ID)
				}

				seen[child.ID] = true
			}

			if len(seen) != n {
				t.Fatalf("total distinct children attached = %d, want %d", len(seen), n)
			}
		})
	}
}

// TestPreloadEmptyParentsSkipsChildQuery proves a parent query that matches
// nothing issues only the parent query -- the child IN query is skipped
// rather than run against an empty id set.
func TestPreloadEmptyParentsSkipsChildQuery(t *testing.T) {
	ctx, conn := newPreloadDB(t)

	spy := &preloadSpy{DB: conn}

	got, err := Preload(
		ctx, spy,
		From(preloadAuthors).Where(preloadAuthorName.Eq("nobody")),
		preloadBookAuthor,
		From(preloadBooks),
		preloadAuthorKey,
		preloadBookFK,
	)
	if err != nil {
		t.Fatalf("Preload: %v", err)
	}

	if len(got) != 0 {
		t.Fatalf("len(got) = %d, want 0", len(got))
	}

	if len(spy.queries) != 1 {
		t.Fatalf("Preload issued %d queries, want exactly 1", len(spy.queries))
	}
}

// TestPreloadErrorPaths proves parent-query and child-query failures
// surface instead of partial results.
func TestPreloadErrorPaths(t *testing.T) {
	ctx := t.Context()
	boom := errors.New("boom")

	parents := From(preloadAuthors)
	children := From(preloadBooks).OrderBy(preloadBookTitle.Asc())

	if _, err := Preload(ctx, fakeDB{}, parents, preloadBookAuthor, children, preloadAuthorKey, preloadBookFK); err == nil {
		t.Fatal("Preload with failing parents succeeded, want an error")
	}

	authorRows := &stubRows{values: [][]any{{"a001", "author-001"}}}

	flaky := &preloadFlakyDB{rows: authorRows, err: boom}

	if _, err := Preload(ctx, flaky, parents, preloadBookAuthor, children, preloadAuthorKey, preloadBookFK); !errors.Is(err, boom) {
		t.Fatalf("Preload err = %v, want errors.Is(err, boom)", err)
	}
}

// preloadFlakyDB serves one fixed row set for the first query (the parents)
// and fails every later query (the children).
type preloadFlakyDB struct {
	mockExec

	calls int
	rows  *stubRows
	err   error
}

func (p *preloadFlakyDB) Dialect() string { return "sqlite" }

func (p *preloadFlakyDB) Query(context.Context, string, ...any) (db.Rows, error) {
	p.calls++

	if p.calls == 1 {
		return p.rows, nil
	}

	return nil, p.err
}
