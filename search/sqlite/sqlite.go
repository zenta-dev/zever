package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"modernc.org/sqlite"

	"github.com/zenta-dev/zever/codec"
	"github.com/zenta-dev/zever/search"
)

var metadataCodec = codec.JSONCodec[map[string]any]{}

// DDL statements, one Exec each.
const (
	schemaDocuments = `CREATE TABLE IF NOT EXISTS search_documents (id TEXT NOT NULL, idx TEXT NOT NULL, content TEXT NOT NULL, metadata BLOB, PRIMARY KEY (id, idx))`
	schemaFTS       = `CREATE VIRTUAL TABLE IF NOT EXISTS search_fts USING fts5(content, id UNINDEXED, idx UNINDEXED)`
)

var memoryCounter uint64

// DefaultDDLTimeout bounds DDL during construction on the local embedded DB; server backends use 10s.
const DefaultDDLTimeout = 5 * time.Second

// Store implements search.Search backed by SQLite FTS5.
type Store struct {
	db *sql.DB
}

// New creates a SQLite-backed Search from Options. An empty DSN selects an
// isolated in-memory database.
func New(o search.Options) (search.Search, error) {
	if err := o.Validate(); err != nil {
		return nil, fmt.Errorf("sqlite: %w", err)
	}

	dsn := o.DSN
	if dsn == "" {
		dsn = ":memory:"
	}

	if dsn == ":memory:" {
		n := atomic.AddUint64(&memoryCounter, 1)
		dsn = fmt.Sprintf("file:zen-search-%d?mode=memory&cache=shared", n)
	} else if err := validateDSN(dsn); err != nil {
		return nil, err
	}

	// NewConnector parses the DSN eagerly, so a malformed DSN fails here
	// with a coverable error; sql.Open would defer every failure to first
	// use and leave this branch untestable.
	connector, err := sqlite.NewConnector(dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open: %w", err)
	}

	db := sql.OpenDB(connector)

	// A single pooled connection serializes access at the Go level. This
	// keeps :memory: shared-cache databases (process-local state tied to
	// whichever connection references them) from racing DDL against
	// themselves on a second pooled connection, and avoids SQLITE_BUSY on
	// concurrent writes without a busy-timeout pragma.
	db.SetMaxOpenConns(1)

	ctx, cancel := context.WithTimeout(context.Background(), DefaultDDLTimeout)
	defer cancel()

	for _, stmt := range []string{schemaDocuments, schemaFTS} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("sqlite: create table: %w", err)
		}
	}

	return &Store{db: db}, nil
}

// Index adds or replaces doc in its index. Metadata is cloned with the
// document content injected under the "content" key, so the caller's map is
// never mutated; a caller-supplied "content" entry is overwritten in the
// clone. The document row and its FTS row are dual-written inside one
// transaction keyed on the documents rowid.
func (s *Store) Index(ctx context.Context, doc search.Document) error {
	if err := doc.Validate(); err != nil {
		return fmt.Errorf("sqlite: index: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: index: %w", err)
	}

	defer func() { _ = tx.Rollback() }()

	if err := indexOne(ctx, tx, doc); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: index: %w", err)
	}

	return nil
}

// IndexBatch adds or replaces all of docs in a single transaction.
func (s *Store) IndexBatch(ctx context.Context, docs []search.Document) error {
	for i, doc := range docs {
		if err := doc.Validate(); err != nil {
			return fmt.Errorf("sqlite: index batch: index %d: %w", i, err)
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: index batch: %w", err)
	}

	defer func() { _ = tx.Rollback() }()

	for _, doc := range docs {
		if err := indexOne(ctx, tx, doc); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: index batch: %w", err)
	}

	return nil
}

// indexOne writes doc's document and FTS rows using tx, so Index and
// IndexBatch share identical per-document logic under either a single- or
// multi-document transaction.
func indexOne(ctx context.Context, tx *sql.Tx, doc search.Document) error {
	meta := make(map[string]any, len(doc.Metadata)+1)
	for k, v := range doc.Metadata {
		meta[k] = v
	}

	meta["content"] = doc.Content

	metaBlob, err := metadataCodec.Encode(meta)
	if err != nil {
		return fmt.Errorf("sqlite: index: %w", err)
	}

	stmts := []struct {
		query string
		args  []any
	}{
		{`DELETE FROM search_fts WHERE id = ? AND idx = ?`, []any{doc.ID, doc.Index}},
		{`INSERT OR REPLACE INTO search_documents (id, idx, content, metadata) VALUES (?, ?, ?, ?)`, []any{doc.ID, doc.Index, doc.Content, metaBlob}},
		{`INSERT INTO search_fts(rowid, content, id, idx) SELECT rowid, ?, ?, ? FROM search_documents WHERE id = ? AND idx = ?`, []any{doc.Content, doc.ID, doc.Index, doc.ID, doc.Index}},
	}

	for _, st := range stmts {
		if _, err := tx.ExecContext(ctx, st.query, st.args...); err != nil {
			return fmt.Errorf("sqlite: index: %w", err)
		}
	}

	return nil
}

// Delete removes every document with id across all indexes in one
// transaction. Cross-index delete by id alone matches the meilisearch
// fan-out semantics: sqlite has no per-index global ID lookup, so the id is
// the only addressing key.
func (s *Store) Delete(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: delete: %w", err)
	}

	defer func() { _ = tx.Rollback() }()

	var total int64

	for _, q := range []string{
		`DELETE FROM search_documents WHERE id = ?`,
		`DELETE FROM search_fts WHERE id = ?`,
	} {
		res, err := tx.ExecContext(ctx, q, id)
		if err != nil {
			return fmt.Errorf("sqlite: delete: %w", err)
		}

		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("sqlite: delete: %w", err)
		}

		total += n
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: delete: %w", err)
	}

	if total == 0 {
		// Pointer chain required: callers match with a *NotFoundError
		// target. The error-typed intermediate keeps that chain while
		// satisfying govet's printf check.
		notFound := error(&search.NotFoundError{ID: id})
		return fmt.Errorf("sqlite: delete: %w", notFound)
	}

	return nil
}

// Search runs query with opts and returns ranked hits. Scores are -bm25
// units (bm25 ranks lower-better, so negation orders higher-first). An empty
// sanitized query returns an empty Result without querying. Without an
// "index" filter the search spans all indexes: sqlite is embedded and
// single-tenant, a documented difference from meilisearch.
func (s *Store) Search(ctx context.Context, query string, opts search.QueryOptions) (search.Result, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = search.DefaultLimit
	}

	offset := opts.Offset
	if offset < 0 {
		offset = 0
	}

	match := buildMatch(query)
	if match == "" {
		return search.Result{}, nil
	}

	where := "search_fts MATCH ?"

	var filter []any

	if index := opts.Filters["index"]; index != "" {
		where += " AND d.idx = ?"
		filter = []any{index}
	}

	hitsSQL := "SELECT d.id, d.metadata, -bm25(search_fts) FROM search_fts f" +
		" JOIN search_documents d ON d.rowid = f.rowid WHERE " + where +
		" ORDER BY bm25(search_fts) LIMIT ? OFFSET ?"
	hitsArgs := append(append([]any{match}, filter...), limit, offset)

	countSQL := "SELECT COUNT(*) FROM search_fts f" +
		" JOIN search_documents d ON d.rowid = f.rowid WHERE " + where
	countArgs := append([]any{match}, filter...)

	rows, err := s.db.QueryContext(ctx, hitsSQL, hitsArgs...)
	if err != nil {
		return search.Result{}, fmt.Errorf("sqlite: search: query: %w", err)
	}

	defer func() { _ = rows.Close() }()

	hits := make([]search.Hit, 0)

	for rows.Next() {
		var id string

		var metaJSON []byte

		var score float64

		if scanErr := rows.Scan(&id, &metaJSON, &score); scanErr != nil {
			return search.Result{}, fmt.Errorf("sqlite: search: scan: %w", scanErr)
		}

		var meta map[string]any

		if metaJSON != nil {
			var unmarshalErr error

			meta, unmarshalErr = metadataCodec.Decode(metaJSON)
			if unmarshalErr != nil {
				return search.Result{}, fmt.Errorf("sqlite: search: scan: %w", unmarshalErr)
			}
		}

		hits = append(hits, search.Hit{ID: id, Score: score, Metadata: meta})
	}

	if rowsErr := rows.Err(); rowsErr != nil {
		return search.Result{}, fmt.Errorf("sqlite: search: rows: %w", rowsErr)
	}

	countRows, countErr := s.db.QueryContext(ctx, countSQL, countArgs...)
	if countErr != nil {
		return search.Result{}, fmt.Errorf("sqlite: search: count: %w", countErr)
	}

	defer func() { _ = countRows.Close() }()

	if !countRows.Next() {
		if countRowsErr := countRows.Err(); countRowsErr != nil {
			return search.Result{}, fmt.Errorf("sqlite: search: count: %w", countRowsErr)
		}

		// Defensive: COUNT(*) always returns exactly one row.
		return search.Result{}, errors.New("sqlite: search: count: no rows")
	}

	var total int64

	if countScanErr := countRows.Scan(&total); countScanErr != nil {
		return search.Result{}, fmt.Errorf("sqlite: search: count: %w", countScanErr)
	}

	return search.Result{Hits: hits, Total: total}, nil
}

// Close releases the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// buildMatch sanitizes query into an FTS5 MATCH expression. Each
// whitespace-separated token is stripped of double quotes (which would
// otherwise break out of phrase quoting or unbalance the expression),
// empty tokens are dropped, and the survivors are wrapped in double quotes
// and joined with spaces. Quoted phrases are literals in FTS5, so quoting
// every token neutralizes keywords (AND/OR/NOT/NEAR), column filters (:),
// and grouping parens; tokens join with implicit AND. A trailing * keeps
// its FTS5 prefix meaning inside quotes. It returns "" when no usable token
// remains; callers must skip querying.
func buildMatch(query string) string {
	var b strings.Builder

	for _, tok := range strings.Fields(query) {
		tok = strings.ReplaceAll(tok, `"`, "")

		if tok == "" {
			continue
		}

		if b.Len() > 0 {
			b.WriteByte(' ')
		}

		b.WriteByte('"')
		b.WriteString(tok)
		b.WriteByte('"')
	}

	return b.String()
}

// validateDSN rejects control characters and statement separators that
// could smuggle extra statements into the DSN. It duplicates the
// vectorstore sqlite backend's check (~10 lines) instead of importing it:
// search must not import vectorstore, and sharing a DSN policy across
// facades would couple their release cycles.
func validateDSN(dsn string) error {
	if strings.Contains(dsn, "\x00") || strings.Contains(dsn, "\n") || strings.Contains(dsn, "\r") {
		return errors.New("sqlite: dsn contains invalid control characters")
	}

	if strings.Contains(dsn, ";") {
		return errors.New("sqlite: dsn must not contain ';'")
	}

	return nil
}
