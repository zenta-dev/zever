package pgvector

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zenta-dev/zever/codec"
	"github.com/zenta-dev/zever/vectorstore"
)

var (
	_ vectorstore.VectorStore = (*Store)(nil)
	_ dbpool                  = (*pgxpool.Pool)(nil)

	metadataCodec  = codec.JSONCodec[map[string]any]{}
	embeddingCodec = codec.JSONCodec[[]float32]{}
)

// dbpool is the narrow query surface Store needs. *pgxpool.Pool satisfies it;
// tests substitute a fake. Splitting DDL into setupStore lets fakes cover every
// branch without a live database.
type dbpool interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Close()
}

// Store implements vectorstore.VectorStore backed by PostgreSQL pgvector.
type Store struct {
	db  dbpool
	dim int
}

// DefaultDDLTimeout bounds connect plus DDL during construction. Shared 10s
// floor with search/postgres: pgvector ivfflat index build slower than plain
// B-tree/GIN; single budget for connect+DDL during construction so they don't
// drift. Server backend; local embedded DB uses 5s.
const DefaultDDLTimeout = 10 * time.Second

// New creates a pgvector Store from Options. It validates Options first, then
// requires a DSN and defaults a non-positive dimension. It connects to the
// DSN and prepares the vectors table and index, capping the pool with
// PoolConfig at DefaultMaxConns. When search, vectorstore, and db share one
// Postgres DSN, keep the sum of per-adapter MaxConns below the server's
// max_connections; for a custom cap, build a config with PoolConfig and wire
// the pool beside this constructor without changing this signature.
func New(o vectorstore.Options) (vectorstore.VectorStore, error) {
	if err := o.Validate(); err != nil {
		return nil, fmt.Errorf("pgvector: %w", err)
	}

	if o.DSN == "" {
		return nil, ErrMissingDSN
	}

	dimension := o.Dimension
	if dimension <= 0 {
		dimension = vectorstore.DefaultDimension
	}

	ddlCtx, cancel := context.WithTimeout(context.Background(), DefaultDDLTimeout)
	defer cancel()

	cfg, err := PoolConfig(o.DSN, DefaultMaxConns)
	if err != nil {
		return nil, err
	}

	pool, err := pgxpool.NewWithConfig(ddlCtx, cfg)
	if err != nil {
		return nil, fmt.Errorf("pgvector: connect: %w", err)
	}

	return setupStore(ddlCtx, pool, dimension)
}

// setupStore runs the extension, inspect, table, and index DDL over dbpool.
// Embeddings use a JSON-string $n::vector cast to avoid a pgvector-go dependency.
func setupStore(ctx context.Context, conn dbpool, dimension int) (*Store, error) {
	if _, err := conn.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS vector`); err != nil {
		conn.Close()
		return nil, fmt.Errorf("pgvector: create extension: %w", err)
	}

	existingDim, exists, err := embeddingColumnDim(ctx, conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("pgvector: inspect embedding column: %w", err)
	}

	if exists {
		if err := checkDimension(existingDim, dimension); err != nil {
			conn.Close()
			return nil, err
		}
	}

	if !exists {
		if _, err := conn.Exec(ctx, tableDDL(dimension)); err != nil {
			conn.Close()
			return nil, fmt.Errorf("pgvector: create table: %w", err)
		}
	}

	if _, err := conn.Exec(ctx,
		`CREATE INDEX IF NOT EXISTS vectors_idx ON vectors USING ivfflat `+
			`(embedding vector_cosine_ops) WITH (lists = 100)`,
	); err != nil {
		conn.Close()
		return nil, fmt.Errorf("pgvector: create index: %w", err)
	}

	return &Store{db: conn, dim: dimension}, nil
}

func embeddingColumnDim(ctx context.Context, conn dbpool) (int, bool, error) {
	rows, err := conn.Query(ctx, `SELECT format_type(a.atttypid, a.atttypmod)
		FROM pg_attribute a
		JOIN pg_class c ON c.oid = a.attrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public'
		  AND c.relname = 'vectors'
		  AND a.attname = 'embedding'
		  AND a.attnum > 0`)
	if err != nil {
		return 0, false, err
	}

	if !rows.Next() {
		if closeErr := rowsCloseErr(rows); closeErr != nil {
			return 0, false, fmt.Errorf("pgvector: rows: %w", closeErr)
		}

		return 0, false, nil
	}

	var typ string
	if scanErr := rows.Scan(&typ); scanErr != nil {
		rows.Close()
		return 0, false, scanErr
	}

	if closeErr := rowsCloseErr(rows); closeErr != nil {
		return 0, false, closeErr
	}

	dim, err := parseVectorType(typ)
	if err != nil {
		return 0, false, err
	}

	return dim, true, nil
}

// checkDimension keeps its domain-specific message verbatim: the text carries
// both numbers greppably, so no typed error wraps it here.
func checkDimension(existing, configured int) error {
	if existing != configured {
		return fmt.Errorf("pgvector: existing vectors.embedding dimension %d does not match configured dimension %d", existing, configured)
	}

	return nil
}

func parseVectorType(typ string) (int, error) {
	const prefix = "vector("
	if !strings.HasPrefix(typ, prefix) || !strings.HasSuffix(typ, ")") {
		return 0, fmt.Errorf("pgvector: unexpected embedding column type %q", typ)
	}

	dim, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(typ, prefix), ")"))
	if err != nil || dim <= 0 {
		return 0, fmt.Errorf("pgvector: unexpected embedding column type %q", typ)
	}

	return dim, nil
}

func tableDDL(dimension int) string {
	if dimension <= 0 {
		dimension = 1536
	}

	return fmt.Sprintf(`CREATE TABLE IF NOT EXISTS vectors (
		id TEXT PRIMARY KEY,
		embedding vector(%d) NOT NULL,
		metadata JSONB
	)`, dimension)
}

// upsertRowCols is the number of bind parameters UpsertBatch's per-row
// placeholder ($n, $n::vector, $n::jsonb) consumes.
const upsertRowCols = 3

// maxUpsertBatchRows caps rows per INSERT statement in UpsertBatch. Postgres's
// extended-protocol wire format allows at most 65535 bind parameters per
// statement; at upsertRowCols (3) params/row that's a hard ceiling around
// 21845 rows. maxUpsertBatchRows keeps 3*20000 = 60000 params per statement,
// a generous margin under the 65535 limit rather than cutting it at the
// boundary.
const maxUpsertBatchRows = 20000

// encodeVectorRow validates vec's embedding dimension and encodes its
// embedding and metadata for storage. Upsert and UpsertBatch both call it so
// a validation-rule change only needs to happen once.
func (s *Store) encodeVectorRow(vec vectorstore.Vector) (vecJSON string, metaJSON []byte, err error) {
	if len(vec.Embedding) != s.dim {
		// Pointer chain required: callers match with a *DimensionMismatchError
		// target. The error-typed intermediate keeps that chain while
		// satisfying govet's printf check.
		mismatch := error(&vectorstore.DimensionMismatchError{Got: len(vec.Embedding), Want: s.dim})
		return "", nil, fmt.Errorf("%w (set the dimension option or recreate the vectors table)", mismatch)
	}

	metaJSON, err = metadataCodec.Encode(vec.Metadata)
	if err != nil {
		return "", nil, fmt.Errorf("marshal metadata: %w", err)
	}

	vecJSONBytes, err := embeddingCodec.Encode(vec.Embedding)
	if err != nil {
		return "", nil, fmt.Errorf("marshal embedding: %w", err)
	}

	return string(vecJSONBytes), metaJSON, nil
}

// Upsert inserts or replaces a vector in the store.
func (s *Store) Upsert(ctx context.Context, vec vectorstore.Vector) error {
	if err := vec.Validate(); err != nil {
		return fmt.Errorf("pgvector: upsert: %w", err)
	}

	vecJSON, metaJSON, err := s.encodeVectorRow(vec)
	if err != nil {
		return fmt.Errorf("pgvector: upsert: %w", err)
	}

	_, err = s.db.Exec(ctx,
		`INSERT INTO vectors (id, embedding, metadata) VALUES ($1, $2::vector, $3::jsonb)
		 ON CONFLICT (id) DO UPDATE SET embedding = EXCLUDED.embedding, metadata = EXCLUDED.metadata`,
		vec.ID, vecJSON, metaJSON,
	)
	if err != nil {
		return fmt.Errorf("pgvector: upsert: %w", err)
	}

	return nil
}

// UpsertBatch inserts or replaces all of vecs, chunked into multi-row INSERT
// statements of at most maxUpsertBatchRows rows each to stay under Postgres's
// bind-parameter limit, still far fewer round trips than one-by-one calls. An
// empty vecs is a no-op.
func (s *Store) UpsertBatch(ctx context.Context, vecs []vectorstore.Vector) error {
	if len(vecs) == 0 {
		return nil
	}

	for i, vec := range vecs {
		if err := vec.Validate(); err != nil {
			return fmt.Errorf("pgvector: upsert batch: index %d: %w", i, err)
		}
	}

	for start := 0; start < len(vecs); start += maxUpsertBatchRows {
		end := start + maxUpsertBatchRows
		if end > len(vecs) {
			end = len(vecs)
		}

		if err := s.upsertBatchChunk(ctx, vecs[start:end]); err != nil {
			return err
		}
	}

	return nil
}

// upsertBatchChunk executes a single multi-row INSERT for chunk, which must
// be small enough to stay under Postgres's bind-parameter limit.
func (s *Store) upsertBatchChunk(ctx context.Context, chunk []vectorstore.Vector) error {
	placeholders := make([]string, 0, len(chunk))
	args := make([]any, 0, len(chunk)*upsertRowCols)

	for i, vec := range chunk {
		vecJSON, metaJSON, err := s.encodeVectorRow(vec)
		if err != nil {
			return fmt.Errorf("pgvector: upsert batch: %w", err)
		}

		base := i * upsertRowCols
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d::vector, $%d::jsonb)", base+1, base+2, base+3))
		args = append(args, vec.ID, vecJSON, metaJSON)
	}

	query := fmt.Sprintf(
		`INSERT INTO vectors (id, embedding, metadata) VALUES %s
		 ON CONFLICT (id) DO UPDATE SET embedding = EXCLUDED.embedding, metadata = EXCLUDED.metadata`,
		strings.Join(placeholders, ", "),
	)

	if _, err := s.db.Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("pgvector: upsert batch: %w", err)
	}

	return nil
}

// Delete removes a vector by ID from the store.
func (s *Store) Delete(ctx context.Context, id string) error {
	// RowsAffected replaces the earlier SELECT-then-DELETE two-round-trip
	// shape with a single Exec, mirroring search/postgres.Delete: zero
	// affected rows means the vector does not exist.
	tag, err := s.db.Exec(ctx, `DELETE FROM vectors WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("pgvector: delete: %w", err)
	}

	if tag.RowsAffected() == 0 {
		// See Upsert: pointer chain required for *NotFoundError targets.
		notFound := error(&vectorstore.NotFoundError{ID: id})
		return fmt.Errorf("pgvector: delete: %w", notFound)
	}

	return nil
}

// Query finds the top-K nearest vectors to the given embedding.
func (s *Store) Query(ctx context.Context, embedding []float32, topK int) ([]vectorstore.ScoreMatch, error) {
	if len(embedding) != s.dim {
		// See Upsert: pointer chain required for *DimensionMismatchError targets.
		mismatch := error(&vectorstore.DimensionMismatchError{Got: len(embedding), Want: s.dim})
		return nil, fmt.Errorf("pgvector: query: %w (set the dimension option or recreate the vectors table)", mismatch)
	}

	vecJSON, err := embeddingCodec.Encode(embedding)
	if err != nil {
		return nil, fmt.Errorf("pgvector: marshal embedding: %w", err)
	}

	if topK <= 0 {
		topK = vectorstore.DefaultTopK
	}

	vecText := string(vecJSON)

	rows, err := s.db.Query(ctx,
		`SELECT id, 1 - (embedding <=> $1::vector) AS score, metadata
		 FROM vectors
		 ORDER BY embedding <=> $2::vector
		 LIMIT $3`,
		vecText, vecText, topK,
	)
	if err != nil {
		return nil, fmt.Errorf("pgvector: query: %w", err)
	}

	var results []vectorstore.ScoreMatch

	for rows.Next() {
		var m vectorstore.ScoreMatch

		// pgx decodes float8 into float64, so scan into float64 then narrow.
		var score float64

		var metaJSON []byte
		if err := rows.Scan(&m.ID, &score, &metaJSON); err != nil {
			rows.Close()
			return nil, fmt.Errorf("pgvector: scan: %w", err)
		}

		m.Score = float32(score)

		if metaJSON != nil {
			var decodeErr error

			m.Metadata, decodeErr = metadataCodec.Decode(metaJSON)
			if decodeErr != nil {
				rows.Close()
				return nil, fmt.Errorf("pgvector: metadata decode: %w", decodeErr)
			}
		}

		results = append(results, m)
	}

	if err := rowsCloseErr(rows); err != nil {
		return nil, fmt.Errorf("pgvector: rows: %w", err)
	}

	return results, nil
}

// Close releases the underlying pool.
func (s *Store) Close() error {
	s.db.Close()

	return nil
}

func rowsCloseErr(rows pgx.Rows) error {
	// pgx closes rows idempotently with no return; Err surfaces iteration
	// errors (the same contract as database/sql's Rows.Err).
	rows.Close()

	return rows.Err()
}
