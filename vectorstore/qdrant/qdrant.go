package qdrant

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/qdrant/go-client/qdrant"

	"github.com/zenta-dev/zever/vectorstore"
)

// Open creates a Qdrant vector store from Options.
// It validates Options first, requires a URL, and connects lazily
// unless a dimension is configured.
func Open(o vectorstore.Options) (vectorstore.VectorStore, error) {
	if err := o.Validate(); err != nil {
		return nil, fmt.Errorf("qdrant: %w", err)
	}

	if o.URL == "" {
		return nil, ErrMissingURL
	}

	return New(o.URL, o.APIKey, o.Dimension)
}

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type pointClient interface {
	Upsert(ctx context.Context, request *qdrant.UpsertPoints) (*qdrant.UpdateResult, error)
	Delete(ctx context.Context, request *qdrant.DeletePoints) (*qdrant.UpdateResult, error)
	Query(ctx context.Context, request *qdrant.QueryPoints) ([]*qdrant.ScoredPoint, error)
	Close() error
	CollectionExists(ctx context.Context, collectionName string) (bool, error)
	CreateCollection(ctx context.Context, request *qdrant.CreateCollection) error
	GetCollectionInfo(ctx context.Context, collectionName string) (*qdrant.CollectionInfo, error)
}

// Store implements vectorstore.VectorStore backed by Qdrant.
type Store struct {
	client  pointClient
	dim     int
	mu      sync.Mutex
	created bool
}

func pointID(id string) *qdrant.PointId {
	if uuidPattern.MatchString(id) {
		return qdrant.NewID(id)
	}

	sum := sha256.Sum256([]byte(id))

	// Deterministic UUIDv5-style generation: use first 16 bytes of SHA-256
	// and set RFC 4122 version (5) and variant bits so the result is a
	// valid UUID. This avoids storing non-UUID hashes that Qdrant rejects.
	var b [16]byte

	copy(b[:], sum[:16])
	b[6] = (b[6] & 0x0f) | 0x50
	b[8] = (b[8] & 0x3f) | 0x80

	return qdrant.NewID(fmt.Sprintf(
		"%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16],
	))
}

// New creates a Qdrant vector store connected to the given address.
func New(addr, apiKey string, dim int) (*Store, error) {
	host, port, useTLS := parseAddr(addr)

	cfg := &qdrant.Config{
		Host:   host,
		Port:   port,
		UseTLS: useTLS,
	}
	if apiKey != "" {
		cfg.APIKey = apiKey
	}

	client, err := qdrant.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("qdrant: client: %w", err)
	}

	s := &Store{client: client, dim: dim}
	if dim > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

		err = s.ensureCollection(ctx, dim)

		cancel()

		if err != nil {
			_ = client.Close() //nolint:gosec // G104: best-effort cleanup; ignore close error

			return nil, err
		}
	}

	return s, nil
}

// Upsert inserts or replaces a vector in the store.
func (s *Store) Upsert(ctx context.Context, vec vectorstore.Vector) error {
	if err := requireEmbedding(vec.Embedding); err != nil {
		return fmt.Errorf("qdrant: upsert: %w", err)
	}

	s.mu.Lock()

	dim := s.dim

	s.mu.Unlock()

	if dim > 0 && len(vec.Embedding) != dim {
		// Pointer chain required for *DimensionMismatchError targets.
		mismatch := error(&vectorstore.DimensionMismatchError{Got: len(vec.Embedding), Want: dim})
		return fmt.Errorf("qdrant: upsert: %w", mismatch)
	}

	if err := s.ensureCollection(ctx, len(vec.Embedding)); err != nil {
		return err
	}

	wait := true

	point, err := newPoint(vec)
	if err != nil {
		return fmt.Errorf("qdrant: upsert: %w", err)
	}

	// Collection name is intentionally hardcoded: the store manages a
	// single "vectors" collection, and a parameter here would risk
	// identifier injection via crafted collection names.
	_, err = s.client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: "vectors",
		Wait:           &wait,
		Points:         []*qdrant.PointStruct{point},
	})
	if err != nil {
		return fmt.Errorf("qdrant: upsert: %w", err)
	}

	return nil
}

func newPoint(vec vectorstore.Vector) (*qdrant.PointStruct, error) {
	// TryValueMap always returns a non-nil map on success (verified against
	// go-client v1.19.0 source), so no nil guard is needed before assignment.
	payload, err := qdrant.TryValueMap(vec.Metadata)
	if err != nil {
		return nil, fmt.Errorf("metadata: %w", err)
	}

	payload["_id"] = qdrant.NewValueString(vec.ID)

	return &qdrant.PointStruct{
		Id:      pointID(vec.ID),
		Vectors: qdrant.NewVectorsDense(vec.Embedding),
		Payload: payload,
	}, nil
}

// Delete removes a vector by ID from the store.
// Delete is idempotent: removing a missing ID reports no error.
func (s *Store) Delete(ctx context.Context, id string) error {
	wait := true

	// Collection name is intentionally hardcoded: the store manages a
	// single "vectors" collection (see Upsert).
	_, err := s.client.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: "vectors",
		Wait:           &wait,
		Points: &qdrant.PointsSelector{
			PointsSelectorOneOf: &qdrant.PointsSelector_Points{
				Points: &qdrant.PointsIdsList{
					Ids: []*qdrant.PointId{pointID(id)},
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("qdrant: delete: %w", err)
	}

	return nil
}

// Query finds the top-K nearest vectors to the given embedding.
func (s *Store) Query(ctx context.Context, embedding []float32, topK int) ([]vectorstore.ScoreMatch, error) {
	if err := requireEmbedding(embedding); err != nil {
		return nil, fmt.Errorf("qdrant: query: %w", err)
	}

	s.mu.Lock()

	dim := s.dim

	s.mu.Unlock()

	if dim > 0 && len(embedding) != dim {
		// Pointer chain required for *DimensionMismatchError targets.
		mismatch := error(&vectorstore.DimensionMismatchError{Got: len(embedding), Want: dim})
		return nil, fmt.Errorf("qdrant: query: %w", mismatch)
	}

	if err := s.ensureCollection(ctx, len(embedding)); err != nil {
		return nil, err
	}

	if topK <= 0 {
		topK = 10
	}

	limit := uint64(topK)

	// Collection name is intentionally hardcoded: the store manages a
	// single "vectors" collection (see Upsert).
	results, err := s.client.Query(ctx, &qdrant.QueryPoints{
		CollectionName: "vectors",
		Query:          qdrant.NewQueryDense(embedding),
		Limit:          &limit,
		WithPayload:    qdrant.NewWithPayloadEnable(true),
	})
	if err != nil {
		return nil, fmt.Errorf("qdrant: query: %w", err)
	}

	out := make([]vectorstore.ScoreMatch, len(results))

	for i, r := range results {
		md := make(map[string]any)
		for k, v := range r.Payload {
			md[k] = extractValue(v)
		}

		id := r.Id.GetUuid()

		if original, ok := md["_id"].(string); ok {
			id = original

			delete(md, "_id")
		}

		out[i] = vectorstore.ScoreMatch{
			ID:       id,
			Score:    r.Score,
			Metadata: md,
		}
	}

	return out, nil
}

// Close releases the underlying Qdrant client.
// Close is best-effort and always reports success.
func (s *Store) Close() error {
	_ = s.client.Close() //nolint:gosec // G104: client close is best-effort

	return nil
}

func (s *Store) ensureCollection(ctx context.Context, n int) error {
	s.mu.Lock()

	if s.created {
		s.mu.Unlock()

		return nil
	}

	dim := collectionDim(s.dim, n)
	s.mu.Unlock()

	// Collection name is intentionally hardcoded: the store manages a
	// single "vectors" collection (see Upsert).
	exists, err := s.client.CollectionExists(ctx, "vectors")
	if err != nil {
		return fmt.Errorf("qdrant: check collection: %w", err)
	}

	if exists {
		info, err := s.client.GetCollectionInfo(ctx, "vectors")
		if err != nil {
			return fmt.Errorf("qdrant: inspect collection: %w", err)
		}

		if info == nil || info.GetConfig() == nil || info.GetConfig().GetParams() == nil ||
			info.GetConfig().GetParams().GetVectorsConfig() == nil {
			return fmt.Errorf("qdrant: collection %q has no vector params", "vectors")
		}

		got := int(info.GetConfig().GetParams().GetVectorsConfig().GetParams().GetSize()) //nolint:gosec // G115: dimension fits in int

		if got != dim {
			return fmt.Errorf(
				"qdrant: collection %q dimension %d does not match configured dimension %d",
				"vectors", got, dim,
			)
		}

		s.mu.Lock()
		s.created = true
		s.mu.Unlock()

		return nil
	}

	// Collection name is intentionally hardcoded: the store manages a
	// single "vectors" collection (see Upsert).
	if err := s.client.CreateCollection(ctx, &qdrant.CreateCollection{
		CollectionName: "vectors",
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     uint64(dim), //nolint:gosec // G115: dimension is positive by design
			Distance: qdrant.Distance_Cosine,
		}),
	}); err != nil {
		// Collection name is intentionally hardcoded (see Upsert): recheck
		// the same collection to tolerate a create race.
		exists, existsErr := s.client.CollectionExists(ctx, "vectors")
		if existsErr != nil || !exists {
			return fmt.Errorf("qdrant: create collection: %w", err)
		}
	}

	s.mu.Lock()
	s.dim = dim
	s.created = true
	s.mu.Unlock()

	return nil
}

func requireEmbedding(embedding []float32) error {
	if len(embedding) == 0 {
		return vectorstore.ErrEmptyEmbedding
	}

	return nil
}

func collectionDim(optDim, embeddingLen int) int {
	if optDim > 0 {
		return optDim
	}

	return embeddingLen
}

func parseAddr(addr string) (string, int, bool) {
	if strings.Contains(addr, "://") {
		u, err := url.Parse(addr)
		if err == nil {
			return hostPort(u)
		}
	}

	return splitHostPort(addr)
}

func hostPort(u *url.URL) (string, int, bool) {
	host := u.Hostname()
	if host == "" {
		host = "localhost"
	}

	tls := u.Scheme == "https"

	port := 6334

	if tls {
		port = 6335
	}

	if p := u.Port(); p != "" {
		if n, err := strconv.Atoi(p); err == nil {
			port = n
		}
	}

	return host, port, tls
}

func splitHostPort(addr string) (string, int, bool) {
	host, port := addr, 6334
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		host = addr[:i]

		if n, err := strconv.Atoi(addr[i+1:]); err == nil {
			port = n
		}
	}

	if host == "" {
		host = "localhost"
	}

	return host, port, false
}

func extractValue(v *qdrant.Value) any {
	switch val := v.GetKind().(type) {
	case *qdrant.Value_StringValue:
		return val.StringValue
	case *qdrant.Value_DoubleValue:
		return val.DoubleValue
	case *qdrant.Value_IntegerValue:
		return val.IntegerValue
	case *qdrant.Value_BoolValue:
		return val.BoolValue
	case *qdrant.Value_ListValue:
		out := make([]any, 0, len(val.ListValue.Values))
		for _, item := range val.ListValue.Values {
			out = append(out, extractValue(item))
		}

		return out

	case *qdrant.Value_StructValue:
		out := make(map[string]any, len(val.StructValue.Fields))
		for k, fv := range val.StructValue.Fields {
			out[k] = extractValue(fv)
		}

		return out
	case *qdrant.Value_NullValue:
		return nil
	default:
		return nil
	}
}
