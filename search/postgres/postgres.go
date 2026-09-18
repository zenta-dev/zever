package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zenta-dev/zever/codec"
	"github.com/zenta-dev/zever/search"
)

var metadataCodec = codec.JSONCodec[map[string]any]{}

const createTable = `CREATE TABLE IF NOT EXISTS search_documents (
	id TEXT NOT NULL,
	idx TEXT NOT NULL,
	content TEXT NOT NULL,
	metadata JSONB,
	PRIMARY KEY (id, idx)
)`

const createIndex = `CREATE INDEX IF NOT EXISTS search_documents_content_idx
	ON search_documents USING GIN (to_tsvector('english', content))`

// dbpool is the narrow pool seam used by postgres. *pgxpool.Pool satisfies it;
// tests substitute scripted fakes so no live database is required.
type dbpool interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Close()
}

var _ dbpool = (*pgxpool.Pool)(nil)

type postgres struct {
	db dbpool
}

// newPool constructs the connection pool. It is a variable (rather than a
// direct pgxpool.New call) so tests can stub the seam without a live database.
// Production pools go through PoolConfig with DefaultMaxConns so adapters
// sharing one DSN stay bounded; see PoolConfig for sharing guidance.
var newPool = func(ctx context.Context, dsn string) (dbpool, error) {
	cfg, err := PoolConfig(dsn, DefaultMaxConns)
	if err != nil {
		return nil, err
	}

	return pgxpool.NewWithConfig(ctx, cfg)
}

// New creates a postgres-backed search.Search.
// An empty DSN returns a dev no-op instance (nil pool) so container
// construction succeeds in dev; every operation on it reports ErrNotConfigured.
// Otherwise a single 5s budget covers connect plus DDL.
//
// Pool guidance: search opens its own pool per New call. When search,
// vectorstore, and db share one Postgres DSN, keep the sum of per-adapter
// MaxConns below the server's max_connections. New uses DefaultMaxConns;
// for a custom cap, build a config with PoolConfig and open the pool beside
// this adapter without changing this signature.
func New(o search.Options) (search.Search, error) {
	if err := o.Validate(); err != nil {
		return nil, fmt.Errorf("postgres: %w", err)
	}

	if o.DSN == "" {
		return &postgres{db: nil}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := newPool(ctx, o.DSN)
	if err != nil {
		return nil, fmt.Errorf("postgres: open: %w", err)
	}

	if _, err := pool.Exec(ctx, createTable); err != nil {
		pool.Close()

		return nil, fmt.Errorf("postgres: create table: %w", err)
	}

	if _, err := pool.Exec(ctx, createIndex); err != nil {
		pool.Close()

		return nil, fmt.Errorf("postgres: create index: %w", err)
	}

	return &postgres{db: pool}, nil
}

// indexRowCols is the number of bind parameters IndexBatch's per-row
// placeholder ($n, $n, $n, $n) consumes.
const indexRowCols = 4

// maxIndexBatchRows caps rows per INSERT statement in IndexBatch. Postgres's
// extended-protocol wire format allows at most 65535 bind parameters per
// statement; at indexRowCols (4) params/row that's a hard ceiling around
// 16383 rows. maxIndexBatchRows keeps 4*15000 = 60000 params per statement, a
// generous margin under the 65535 limit rather than cutting it at the
// boundary.
const maxIndexBatchRows = 15000

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

// Index adds or replaces doc in its index.
func (p *postgres) Index(ctx context.Context, doc search.Document) error {
	if p.db == nil {
		return ErrNotConfigured
	}

	metaJSON, err := encodeDocumentMetadata(doc)
	if err != nil {
		return fmt.Errorf("postgres: index: %w", err)
	}

	_, err = p.db.Exec(ctx,
		`INSERT INTO search_documents (id, idx, content, metadata)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (id, idx) DO UPDATE SET content = $5, metadata = $6`,
		doc.ID, doc.Index, doc.Content, metaJSON, doc.Content, metaJSON)
	if err != nil {
		return fmt.Errorf("postgres: index: %w", err)
	}

	return nil
}

// IndexBatch adds or replaces all of docs, chunked into multi-row INSERT
// statements of at most maxIndexBatchRows rows each to stay under Postgres's
// bind-parameter limit, still far fewer round trips than one-by-one calls. An
// empty docs is a no-op.
func (p *postgres) IndexBatch(ctx context.Context, docs []search.Document) error {
	if p.db == nil {
		return ErrNotConfigured
	}

	if len(docs) == 0 {
		return nil
	}

	for start := 0; start < len(docs); start += maxIndexBatchRows {
		end := start + maxIndexBatchRows
		if end > len(docs) {
			end = len(docs)
		}

		if err := p.indexBatchChunk(ctx, docs[start:end]); err != nil {
			return err
		}
	}

	return nil
}

// indexBatchChunk executes a single multi-row INSERT for chunk, which must
// be small enough to stay under Postgres's bind-parameter limit.
func (p *postgres) indexBatchChunk(ctx context.Context, chunk []search.Document) error {
	placeholders := make([]string, 0, len(chunk))
	args := make([]any, 0, len(chunk)*indexRowCols)

	for i, doc := range chunk {
		metaJSON, err := encodeDocumentMetadata(doc)
		if err != nil {
			return fmt.Errorf("postgres: index batch: %w", err)
		}

		base := i * indexRowCols
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d)", base+1, base+2, base+3, base+4))
		args = append(args, doc.ID, doc.Index, doc.Content, metaJSON)
	}

	query := fmt.Sprintf(
		`INSERT INTO search_documents (id, idx, content, metadata) VALUES %s
		 ON CONFLICT (id, idx) DO UPDATE SET content = EXCLUDED.content, metadata = EXCLUDED.metadata`,
		strings.Join(placeholders, ", "),
	)

	if _, err := p.db.Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("postgres: index batch: %w", err)
	}

	return nil
}

// Delete removes the document with id.
func (p *postgres) Delete(ctx context.Context, id string) error {
	if p.db == nil {
		return ErrNotConfigured
	}

	// RowsAffected replaces the RETURNING round trip with identical behavior:
	// zero affected rows means the document does not exist.
	tag, err := p.db.Exec(ctx, `DELETE FROM search_documents WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("postgres: delete: %w", err)
	}

	if tag.RowsAffected() == 0 {
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

// Search runs query with opts and returns ranked hits.
func (p *postgres) Search(ctx context.Context, query string, opts search.QueryOptions) (search.Result, error) {
	if p.db == nil {
		return search.Result{}, ErrNotConfigured
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = search.DefaultLimit
	}

	offset := opts.Offset
	if offset < 0 {
		offset = 0
	}

	hitsSQL, countSQL, hitsArgs, countArgs := buildSearchQueries(query, opts.Filters, limit, offset)

	rows, err := p.db.Query(ctx, hitsSQL, hitsArgs...)
	if err != nil {
		return search.Result{}, fmt.Errorf("postgres: search: %w", err)
	}

	defer rows.Close()

	var hits []search.Hit

	for rows.Next() {
		var id string

		var metaJSON []byte

		var score float64

		if err = rows.Scan(&id, &metaJSON, &score); err != nil {
			return search.Result{}, fmt.Errorf("postgres: search scan: %w", err)
		}

		var meta map[string]any

		if metaJSON != nil {
			if meta, err = metadataCodec.Decode(metaJSON); err != nil {
				return search.Result{}, fmt.Errorf("postgres: search scan: %w", err)
			}
		}

		hits = append(hits, search.Hit{ID: id, Score: score, Metadata: meta})
	}

	if err = rows.Err(); err != nil {
		return search.Result{}, fmt.Errorf("postgres: search: %w", err)
	}

	countRows, err := p.db.Query(ctx, countSQL, countArgs...)
	if err != nil {
		return search.Result{}, fmt.Errorf("postgres: search count: %w", err)
	}

	defer countRows.Close()

	if !countRows.Next() {
		if err := countRows.Err(); err != nil {
			return search.Result{}, fmt.Errorf("postgres: search count: %w", err)
		}

		// Defensive: COUNT(*) always returns exactly one row.
		return search.Result{}, fmt.Errorf("postgres: search count: no rows") //nolint:perfsprint // spec-mandated error form
	}

	var total int64
	if err := countRows.Scan(&total); err != nil {
		return search.Result{}, fmt.Errorf("postgres: search count: %w", err)
	}

	return search.Result{Hits: hits, Total: total}, nil
}

// Close releases backend resources. Unlike pgx rows, pool Close is void, so
// there are no close-error branches.
func (p *postgres) Close() error {
	if p.db == nil {
		return nil
	}

	p.db.Close()

	return nil
}
