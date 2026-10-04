package db

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	dbpostgres "github.com/zenta-dev/zever/adapters/db/postgres"
	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/search"
	"github.com/zenta-dev/zever/orm"
	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/shared/codec"
	"github.com/zenta-dev/zever/shared/dbconn"
)

var metadataCodec = codec.JSONCodec[map[string]any]{}

// docRow is the search_documents entity. Column order matches docColumns:
// the positional Scan must read them in exactly this order.
type docRow struct {
	ID       string
	Index    string
	Content  string
	Metadata []byte
}

// docColumns is the entity column list in Scan order.
var docColumns = []string{"id", "idx", "content", "metadata"}

// Scan reads one row positionally, coercing driver representations:
// metadata arrives as []byte (postgres JSONB, sqlite BLOB) or string.
func (r *docRow) Scan(row orm.Row) error {
	var id, index, content string

	var metaRaw any

	if err := row.Scan(&id, &index, &content, &metaRaw); err != nil {
		return err
	}

	meta, err := coerceMeta(metaRaw)
	if err != nil {
		return err
	}

	*r = docRow{ID: id, Index: index, Content: content, Metadata: meta}

	return nil
}

// coerceMeta converts a metadata cell to raw JSON bytes.
func coerceMeta(v any) ([]byte, error) {
	switch t := v.(type) {
	case nil:
		return nil, nil
	case []byte:
		return t, nil
	case string:
		return []byte(t), nil
	default:
		return nil, fmt.Errorf("postgres: unsupported metadata %T: %w", v, search.ErrInvalidMetadata)
	}
}

// driver is a DB-backed search.Search. It is safe for concurrent use.
type driver struct {
	conn   coredb.DB
	tbl    orm.Table[docRow]
	cID    orm.Column[docRow, string]
	cIdx   orm.Column[docRow, string]
	cBody  orm.Column[docRow, string]
	cMeta  orm.Column[docRow, []byte]
	owns   bool
	closed atomic.Bool
}

var _ search.Search = (*driver)(nil)

// New creates a DB-backed search.Search. Empty DSN selects sqlite at Path
// (default ":memory:"); a postgres URL DSN opens postgres; any other
// non-empty DSN is a sqlite file path. The schema (documents table plus the
// postgres GIN index or the sqlite FTS5 index) is created when missing. The
// driver owns its connection: Close releases it.
func New(o Options) (search.Search, error) {
	return Open(o)
}

// Open creates a DB-backed search.Search; see New.
func Open(o Options) (search.Search, error) {
	if err := o.Validate(); err != nil {
		return nil, err
	}

	poolOpts := o.Options

	var (
		conn coredb.DB
		err  error
	)

	switch {
	case strings.TrimSpace(poolOpts.DSN) == "" && poolOpts.Path == "":
		poolOpts.Path = ":memory:"
		conn, err = dbsqlite.New(poolOpts)
	case dbconn.IsPostgresDSN(poolOpts.DSN):
		conn, err = dbpostgres.New(poolOpts)
	case strings.TrimSpace(poolOpts.DSN) != "":
		poolOpts.Path = poolOpts.DSN
		poolOpts.DSN = ""
		conn, err = dbsqlite.New(poolOpts)
	default:
		conn, err = dbsqlite.New(poolOpts)
	}

	if err != nil {
		return nil, err
	}

	d, err := openFromDB(conn, o, true)
	if err != nil {
		_ = conn.Close(context.Background())

		return nil, err
	}

	return d, nil
}

// NewFromDB creates a DB-backed search.Search over an already-open
// coredb.DB, skipping DSN/Path construction. The caller retains ownership
// of db: Close on the returned Search does not close db, and a failed
// NewFromDB never closes db. Only sqlite and postgres dialects are
// supported; anything else fails closed.
func NewFromDB(conn coredb.DB, o Options) (search.Search, error) {
	if conn == nil {
		return nil, ErrNilDB
	}

	if err := o.Validate(); err != nil {
		return nil, err
	}

	return openFromDB(conn, o, false)
}

// OpenFromDB creates a DB-backed search.Search over an already-open
// coredb.DB; see NewFromDB.
func OpenFromDB(conn coredb.DB, o Options) (search.Search, error) {
	return NewFromDB(conn, o)
}

// openFromDB pings conn, ensures the schema, and wires the driver. owns
// reports whether the driver owns conn and may close it in Close.
func openFromDB(conn coredb.DB, _ Options, owns bool) (search.Search, error) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultConnectTimeout)
	defer cancel()

	if err := conn.Ping(ctx); err != nil {
		return nil, err
	}

	const table = "search_documents"

	d := &driver{
		conn:  conn,
		tbl:   orm.NewTable[docRow](table, docColumns),
		cID:   orm.NewColumn[docRow, string](table, "id"),
		cIdx:  orm.NewColumn[docRow, string](table, "idx"),
		cBody: orm.NewColumn[docRow, string](table, "content"),
		cMeta: orm.NewColumn[docRow, []byte](table, "metadata"),
		owns:  owns,
	}

	if err := d.ensureSchema(ctx); err != nil {
		return nil, err
	}

	return d, nil
}

// checkDialect fails closed unless the connection's dialect resolves to a
// FullTextDialect with a supported flavor (tsvector or FTS5). An
// unresolvable dialect name, or a dialect with neither flavor, is rejected
// with the capability error, never a silently-degraded ranking.
func (d *driver) checkDialect() error {
	dd, err := dialect.For(d.conn.Dialect())
	if err != nil {
		return fmt.Errorf("orm: postgres: unsupported dialect %q: %w",
			d.conn.Dialect(), dialect.ErrUnsupportedByDialect)
	}

	ft, ok := dd.(dialect.FullTextDialect)
	if !ok {
		return fmt.Errorf("orm: postgres: unsupported dialect %q: %w",
			d.conn.Dialect(), dialect.ErrUnsupportedByDialect)
	}

	if !ft.SupportsTSVector() && !ft.SupportsFTS5() {
		return fmt.Errorf("orm: postgres: unsupported dialect %q: %w",
			d.conn.Dialect(), dialect.ErrUnsupportedByDialect)
	}

	return nil
}

// ensureSchema creates the documents table plus the dialect full-text
// index when missing. DDL only: writes below go through the orm typed
// builder, and ranked reads use dialect-parameterized SELECTs (the orm
// query API cannot project the ts_rank/bm25 score expression alongside
// rows -- see orm/fts/postgres's Rank limitation note).
func (d *driver) ensureSchema(ctx context.Context) error {
	if err := d.checkDialect(); err != nil {
		return err
	}

	if d.conn.Dialect() == "postgres" {
		for _, ddl := range []string{
			`CREATE TABLE IF NOT EXISTS search_documents (` +
				`id TEXT NOT NULL, ` +
				`idx TEXT NOT NULL, ` +
				`content TEXT NOT NULL, ` +
				`metadata JSONB, ` +
				`PRIMARY KEY (id, idx))`,
			`CREATE INDEX IF NOT EXISTS search_documents_content_idx ` +
				`ON search_documents USING GIN (to_tsvector('english', content))`,
		} {
			if _, err := d.conn.Exec(ctx, ddl); err != nil {
				return fmt.Errorf("postgres: ensure schema: %w", err)
			}
		}
	} else {
		// External-content FTS5 plus triggers is the documented
		// keep-in-sync shape: writes touch only search_documents (via
		// orm) and the triggers mirror them into search_fts.
		for _, ddl := range []string{
			`CREATE TABLE IF NOT EXISTS search_documents (` +
				`id TEXT NOT NULL, ` +
				`idx TEXT NOT NULL, ` +
				`content TEXT NOT NULL, ` +
				`metadata BLOB, ` +
				`PRIMARY KEY (id, idx))`,
			`CREATE VIRTUAL TABLE IF NOT EXISTS search_fts USING fts5(` +
				`content, id UNINDEXED, idx UNINDEXED, ` +
				`content='search_documents', content_rowid='rowid')`,
			`CREATE TRIGGER IF NOT EXISTS search_fts_ai AFTER INSERT ON search_documents BEGIN ` +
				`INSERT INTO search_fts(rowid, content, id, idx) ` +
				`VALUES (new.rowid, new.content, new.id, new.idx); END`,
			`CREATE TRIGGER IF NOT EXISTS search_fts_ad AFTER DELETE ON search_documents BEGIN ` +
				`INSERT INTO search_fts(search_fts, rowid, content, id, idx) ` +
				`VALUES ('delete', old.rowid, old.content, old.id, old.idx); END`,
			`CREATE TRIGGER IF NOT EXISTS search_fts_au AFTER UPDATE ON search_documents BEGIN ` +
				`INSERT INTO search_fts(search_fts, rowid, content, id, idx) ` +
				`VALUES ('delete', old.rowid, old.content, old.id, old.idx); ` +
				`INSERT INTO search_fts(rowid, content, id, idx) ` +
				`VALUES (new.rowid, new.content, new.id, new.idx); END`,
		} {
			if _, err := d.conn.Exec(ctx, ddl); err != nil {
				return fmt.Errorf("postgres: ensure schema: %w", err)
			}
		}
	}

	return nil
}

// encodeDocumentMetadata clones doc's metadata with its content injected
// under the "content" key (without mutating the caller's map) and encodes
// it. Index and IndexBatch both call it so a validation/encoding-rule change
// only needs to happen once.
func encodeDocumentMetadata(doc search.Document) ([]byte, error) {
	meta := make(map[string]any, len(doc.Metadata)+1)
	for k, v := range doc.Metadata {
		meta[k] = v
	}

	meta["content"] = doc.Content

	return metadataCodec.Encode(meta)
}

// upsertOne writes one document with INSERT ... ON CONFLICT (id, idx)
// DO UPDATE so re-indexing replaces the row. Batch callers loop it: the
// typed builder has no EXCLUDED reference, so one shared DO UPDATE SET list
// cannot carry per-row replacement values for a multi-row statement.
func (d *driver) upsertOne(ctx context.Context, op string, doc search.Document, metaJSON []byte) error {
	err := orm.InsertInto(d.tbl).Values(
		orm.Set(d.cID, doc.ID),
		orm.Set(d.cIdx, doc.Index),
		orm.Set(d.cBody, doc.Content),
		orm.Set(d.cMeta, metaJSON),
	).OnConflict(d.cID.Col(), d.cIdx.Col()).DoUpdate(
		orm.Set(d.cBody, doc.Content),
		orm.Set(d.cMeta, metaJSON),
	).Exec(ctx, d.conn)
	if err != nil {
		return fmt.Errorf("postgres: %s: %w", op, err)
	}

	return nil
}

// Index adds or replaces doc in its index.
func (d *driver) Index(ctx context.Context, doc search.Document) error {
	if err := d.checkDialect(); err != nil {
		return err
	}

	if err := doc.Validate(); err != nil {
		return fmt.Errorf("postgres: index: %w", err)
	}

	metaJSON, err := encodeDocumentMetadata(doc)
	if err != nil {
		return fmt.Errorf("postgres: index: %w", err)
	}

	return d.upsertOne(ctx, "index", doc, metaJSON)
}

// IndexBatch adds or replaces all of docs, one typed upsert per document
// (see upsertOne). An empty docs is a no-op.
func (d *driver) IndexBatch(ctx context.Context, docs []search.Document) error {
	if err := d.checkDialect(); err != nil {
		return err
	}

	if len(docs) == 0 {
		return nil
	}

	for i, doc := range docs {
		if err := doc.Validate(); err != nil {
			return fmt.Errorf("postgres: index batch: index %d: %w", i, err)
		}
	}

	for i, doc := range docs {
		metaJSON, err := encodeDocumentMetadata(doc)
		if err != nil {
			return fmt.Errorf("postgres: index batch: index %d: %w", i, err)
		}

		if err := d.upsertOne(ctx, "index batch", doc, metaJSON); err != nil {
			return err
		}
	}

	return nil
}

// Delete removes the document with id.
func (d *driver) Delete(ctx context.Context, id string) error {
	if err := d.checkDialect(); err != nil {
		return err
	}

	// RowsAffected replaces the RETURNING round trip with identical behavior:
	// zero affected rows means the document does not exist.
	n, err := orm.DeleteFrom(d.tbl).Where(d.cID.Eq(id)).Exec(ctx, d.conn)
	if err != nil {
		return fmt.Errorf("postgres: delete: %w", err)
	}

	if n == 0 {
		// notFound is typed as error first: go vet's printf check rejects
		// %w with *NotFoundError directly (value-receiver Error method);
		// same pattern as search/meilisearch.
		notFound := error(&search.NotFoundError{ID: id})

		return fmt.Errorf("postgres: delete: %w", notFound)
	}

	return nil
}

func matchWhere(query string, filters map[string]string) (string, []any) {
	args := make([]any, 0, 2)
	args = append(args, query)

	var sb strings.Builder

	sb.WriteString(" WHERE to_tsvector('english', content)")
	sb.WriteString(" @@ plainto_tsquery('english', $1)")

	if idx, ok := filters["index"]; ok && idx != "" {
		sb.WriteString(" AND idx = $2")

		args = append(args, idx)
	}

	return sb.String(), args
}

func buildSearchQueries(
	query string, filters map[string]string, limit, offset int,
) (hitsSQL, countSQL string, hitsArgs, countArgs []any) {
	where, whereArgs := matchWhere(query, filters)
	countSQL = `SELECT COUNT(*) FROM search_documents` + where
	countArgs = whereArgs

	// The rank expression reuses the $1 query binding, so hits args are the
	// where args plus limit/offset numbered from len(whereArgs)+1.
	hitsArgs = make([]any, 0, len(whereArgs)+2)
	hitsArgs = append(hitsArgs, whereArgs...)
	hitsArgs = append(hitsArgs, limit, offset)

	var sb strings.Builder

	sb.WriteString(
		`SELECT id, metadata, ` +
			`ts_rank(to_tsvector('english', content),` +
			` plainto_tsquery('english', $1)) AS score` +
			` FROM search_documents`,
	)
	sb.WriteString(where)
	fmt.Fprintf(&sb, ` ORDER BY score DESC LIMIT $%d OFFSET $%d`, len(whereArgs)+1, len(whereArgs)+2)

	return sb.String(), countSQL, hitsArgs, countArgs
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

// Search runs query with opts and returns ranked hits.
func (d *driver) Search(ctx context.Context, query string, opts search.QueryOptions) (search.Result, error) {
	if err := d.checkDialect(); err != nil {
		return search.Result{}, err
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = search.DefaultLimit
	}

	offset := opts.Offset
	if offset < 0 {
		offset = 0
	}

	if d.conn.Dialect() == "postgres" {
		return d.searchPostgres(ctx, query, opts.Filters, limit, offset)
	}

	return d.searchSQLite(ctx, query, opts.Filters, limit, offset)
}

// searchPostgres runs the tsvector ranked query plus its COUNT(*) twin.
func (d *driver) searchPostgres(ctx context.Context, query string, filters map[string]string, limit, offset int) (search.Result, error) {
	hitsSQL, countSQL, hitsArgs, countArgs := buildSearchQueries(query, filters, limit, offset)

	rows, err := d.conn.Query(ctx, hitsSQL, hitsArgs...)
	if err != nil {
		return search.Result{}, fmt.Errorf("postgres: search: %w", err)
	}

	defer func() { _ = rows.Close() }()

	var hits []search.Hit

	for rows.Next() {
		var id string

		var metaRaw any

		var score float64

		if err = rows.Scan(&id, &metaRaw, &score); err != nil {
			return search.Result{}, fmt.Errorf("postgres: search scan: %w", err)
		}

		meta, derr := decodeHitMeta(metaRaw)
		if derr != nil {
			return search.Result{}, fmt.Errorf("postgres: search scan: %w", derr)
		}

		hits = append(hits, search.Hit{ID: id, Score: score, Metadata: meta})
	}

	if err = rows.Err(); err != nil {
		return search.Result{}, fmt.Errorf("postgres: search: %w", err)
	}

	total, terr := d.searchCount(ctx, countSQL, countArgs)
	if terr != nil {
		return search.Result{}, terr
	}

	return search.Result{Hits: hits, Total: total}, nil
}

// searchSQLite runs the FTS5 ranked query plus its COUNT(*) twin. Scores
// are -bm25 units (bm25 ranks lower-better, so negation orders
// higher-first). An empty sanitized query returns an empty Result without
// querying. Without an "index" filter the search spans all indexes: the
// embedded DB is single-tenant, a documented difference from meilisearch.
func (d *driver) searchSQLite(ctx context.Context, query string, filters map[string]string, limit, offset int) (search.Result, error) {
	match := buildMatch(query)
	if match == "" {
		return search.Result{}, nil
	}

	where := "search_fts MATCH ?"

	var filter []any

	if index := filters["index"]; index != "" {
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

	rows, err := d.conn.Query(ctx, hitsSQL, hitsArgs...)
	if err != nil {
		return search.Result{}, fmt.Errorf("postgres: search: query: %w", err)
	}

	defer func() { _ = rows.Close() }()

	hits := make([]search.Hit, 0)

	for rows.Next() {
		var id string

		var metaRaw any

		var score float64

		if scanErr := rows.Scan(&id, &metaRaw, &score); scanErr != nil {
			return search.Result{}, fmt.Errorf("postgres: search: scan: %w", scanErr)
		}

		meta, unmarshalErr := decodeHitMeta(metaRaw)
		if unmarshalErr != nil {
			return search.Result{}, fmt.Errorf("postgres: search: scan: %w", unmarshalErr)
		}

		hits = append(hits, search.Hit{ID: id, Score: score, Metadata: meta})
	}

	if rowsErr := rows.Err(); rowsErr != nil {
		return search.Result{}, fmt.Errorf("postgres: search: rows: %w", rowsErr)
	}

	total, err := d.searchCount(ctx, countSQL, countArgs)
	if err != nil {
		return search.Result{}, err
	}

	return search.Result{Hits: hits, Total: total}, nil
}

// searchCount runs a COUNT(*) twin query shared by both dialect reads.
func (d *driver) searchCount(ctx context.Context, countSQL string, countArgs []any) (int64, error) {
	countRows, err := d.conn.Query(ctx, countSQL, countArgs...)
	if err != nil {
		return 0, fmt.Errorf("postgres: search count: %w", err)
	}

	defer func() { _ = countRows.Close() }()

	if !countRows.Next() {
		if err := countRows.Err(); err != nil {
			return 0, fmt.Errorf("postgres: search count: %w", err)
		}

		// Defensive: COUNT(*) always returns exactly one row.
		return 0, fmt.Errorf("postgres: search count: no rows: %w", search.ErrNotFound)
	}

	var total int64
	if err := countRows.Scan(&total); err != nil {
		return 0, fmt.Errorf("postgres: search count: %w", err)
	}

	return total, nil
}

// decodeHitMeta decodes one hit's stored metadata JSON.
func decodeHitMeta(metaRaw any) (map[string]any, error) {
	metaJSON, err := coerceMeta(metaRaw)
	if err != nil {
		return nil, err
	}

	var meta map[string]any

	if metaJSON != nil {
		if meta, err = metadataCodec.Decode(metaJSON); err != nil {
			return nil, err
		}
	}

	return meta, nil
}

// Close releases the database pool when this driver owns its connection
// (built via New/Open). Drivers built via NewFromDB/OpenFromDB borrow the
// caller's DB and Close is a no-op. Close is idempotent.
func (d *driver) Close() error {
	if !d.closed.CompareAndSwap(false, true) {
		return nil
	}

	if !d.owns {
		return nil
	}

	return d.conn.Close(context.Background())
}
