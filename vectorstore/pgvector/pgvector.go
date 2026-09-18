package pgvector

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zenta-dev/zever/vectorstore"
)

var (
	_ vectorstore.VectorStore = (*Store)(nil)
	_ dbpool                  = (*pgxpool.Pool)(nil)
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

// Open creates a pgvector Store from Options. It validates Options first, then
// requires a DSN and defaults a non-positive dimension.
func Open(o vectorstore.Options) (vectorstore.VectorStore, error) {
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

	return New(o.DSN, dimension)
}

// New connects to the DSN and prepares the vectors table and index.
// It caps the pool with PoolConfig at DefaultMaxConns. When search,
// vectorstore, and db share one Postgres DSN, keep the sum of per-adapter
// MaxConns below the server's max_connections; for a custom cap, build a
// config with PoolConfig and wire the pool beside this constructor without
// changing this signature.
func New(dsn string, dimension int) (*Store, error) {
	if dimension <= 0 {
		dimension = vectorstore.DefaultDimension
	}

	ddlCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cfg, err := PoolConfig(dsn, DefaultMaxConns)
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
			return nil, fmt.Errorf("pgvector: %w", err)
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
		return fmt.Errorf("existing vectors.embedding dimension %d does not match configured dimension %d", existing, configured)
	}

	return nil
}

func parseVectorType(typ string) (int, error) {
	const prefix = "vector("
	if !strings.HasPrefix(typ, prefix) || !strings.HasSuffix(typ, ")") {
		return 0, fmt.Errorf("unexpected embedding column type %q", typ)
	}

	dim, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(typ, prefix), ")"))
	if err != nil || dim <= 0 {
		return 0, fmt.Errorf("unexpected embedding column type %q", typ)
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

// Upsert inserts or replaces a vector in the store.
func (s *Store) Upsert(ctx context.Context, vec vectorstore.Vector) error {
	if len(vec.Embedding) != s.dim {
		// Pointer chain required: callers match with a *DimensionMismatchError
		// target. The error-typed intermediate keeps that chain while
		// satisfying govet's printf check.
		mismatch := error(&vectorstore.DimensionMismatchError{Got: len(vec.Embedding), Want: s.dim})
		return fmt.Errorf("pgvector: upsert: %w (set the dimension option or recreate the vectors table)", mismatch)
	}

	metaJSON, err := json.Marshal(vec.Metadata)
	if err != nil {
		return fmt.Errorf("pgvector: marshal metadata: %w", err)
	}

	vecJSON, err := json.Marshal(vec.Embedding)
	if err != nil {
		return fmt.Errorf("pgvector: marshal embedding: %w", err)
	}

	_, err = s.db.Exec(ctx,
		`INSERT INTO vectors (id, embedding, metadata) VALUES ($1, $2::vector, $3::jsonb)
		 ON CONFLICT (id) DO UPDATE SET embedding = EXCLUDED.embedding, metadata = EXCLUDED.metadata`,
		vec.ID, string(vecJSON), metaJSON,
	)
	if err != nil {
		return fmt.Errorf("pgvector: upsert: %w", err)
	}

	return nil
}

// Delete removes a vector by ID from the store.
func (s *Store) Delete(ctx context.Context, id string) error {
	rows, err := s.db.Query(ctx, `SELECT 1 FROM vectors WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("pgvector: delete: %w", err)
	}

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			rows.Close()
			return fmt.Errorf("pgvector: delete: %w", err)
		}

		rows.Close()

		// See Upsert: pointer chain required for *NotFoundError targets.
		notFound := error(&vectorstore.NotFoundError{ID: id})
		return fmt.Errorf("pgvector: delete: %w", notFound)
	}

	rows.Close()

	if _, err := s.db.Exec(ctx, `DELETE FROM vectors WHERE id = $1`, id); err != nil {
		return fmt.Errorf("pgvector: delete: %w", err)
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

	vecJSON, err := json.Marshal(embedding)
	if err != nil {
		return nil, fmt.Errorf("pgvector: marshal embedding: %w", err)
	}

	if topK <= 0 {
		topK = vectorstore.DefaultTopK
	}

	rows, err := s.db.Query(ctx,
		`SELECT id, 1 - (embedding <=> $1::vector) AS score, metadata
		 FROM vectors
		 ORDER BY embedding <=> $2::vector
		 LIMIT $3`,
		string(vecJSON), string(vecJSON), topK,
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
			if err := json.Unmarshal(metaJSON, &m.Metadata); err != nil {
				rows.Close()
				return nil, fmt.Errorf("pgvector: metadata decode: %w", err)
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

func formatVector(e []float32) string {
	if len(e) == 0 {
		return "[]"
	}

	b, _ := json.Marshal(e)

	return string(b)
}

func rowsCloseErr(rows pgx.Rows) error {
	// pgx closes rows idempotently with no return; Err surfaces iteration
	// errors (the same contract as database/sql's Rows.Err).
	rows.Close()

	return rows.Err()
}
