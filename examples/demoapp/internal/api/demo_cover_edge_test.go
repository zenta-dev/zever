package api_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	authjwt "github.com/zenta-dev/zever/adapters/auth/jwt"
	logslog "github.com/zenta-dev/zever/adapters/log/slog"
	passwordargon2 "github.com/zenta-dev/zever/adapters/password/argon2"
	routerstdhttp "github.com/zenta-dev/zever/adapters/router/stdhttp"
	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/core/ai"
	"github.com/zenta-dev/zever/core/auth"
	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/document"
	"github.com/zenta-dev/zever/core/eventbus"
	"github.com/zenta-dev/zever/core/idempotency"
	"github.com/zenta-dev/zever/core/lock"
	"github.com/zenta-dev/zever/core/media"
	"github.com/zenta-dev/zever/core/notification"
	"github.com/zenta-dev/zever/core/observability"
	"github.com/zenta-dev/zever/core/permission"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/core/ratelimit"
	"github.com/zenta-dev/zever/core/search"
	"github.com/zenta-dev/zever/core/session"
	"github.com/zenta-dev/zever/core/storage"
	"github.com/zenta-dev/zever/core/vectorstore"
	"github.com/zenta-dev/zever/core/workflow"
	"github.com/zenta-dev/zever/examples/demoapp/internal/api"
)

// errBoom is the generic backend failure stubs report.
var errBoom = errors.New("boom")

// stubFailDB fails every query and exec. Handlers must answer 4xx/5xx,
// never panic, on a broken database.
type stubFailDB struct{}

func (stubFailDB) Query(context.Context, string, ...any) (db.Rows, error) {
	return nil, errBoom
}
func (stubFailDB) Exec(context.Context, string, ...any) (int64, error) {
	return 0, errBoom
}
func (stubFailDB) Ping(context.Context) error  { return nil }
func (stubFailDB) Close(context.Context) error { return nil }
func (stubFailDB) Dialect() string             { return "sqlite" }

// stubCache fails or succeeds cache calls on demand.
type stubCache struct {
	getErr error
	setErr error
	val    []byte
}

func (s stubCache) Get(context.Context, string) ([]byte, error) { return s.val, s.getErr }
func (s stubCache) Set(context.Context, string, []byte, time.Duration) error {
	return s.setErr
}
func (stubCache) SetIfAbsent(context.Context, string, []byte, time.Duration) (bool, error) {
	return true, nil
}
func (stubCache) Delete(context.Context, string) error         { return nil }
func (stubCache) Increment(context.Context, string) error      { return nil }
func (stubCache) Decrement(context.Context, string) error      { return nil }
func (stubCache) Exists(context.Context, string) (bool, error) { return true, nil }
func (stubCache) Close(context.Context) error                  { return nil }

// stubFlag returns scripted flag values and errors.
type stubFlag struct {
	boolVal bool
	boolErr error
	strVal  string
	strErr  error
}

func (s stubFlag) Bool(context.Context, string, bool) (bool, error)       { return s.boolVal, s.boolErr }
func (s stubFlag) String(context.Context, string, string) (string, error) { return s.strVal, s.strErr }
func (stubFlag) Int(context.Context, string, int) (int, error)            { return 0, nil }
func (stubFlag) JSON(context.Context, string, any, any) error             { return nil }
func (stubFlag) Close() error                                             { return nil }

// stubPerm answers permission checks with a scripted decision or error.
type stubPerm struct {
	allowed bool
	err     error
}

func (s stubPerm) Can(context.Context, permission.Subject, string, permission.Resource) (permission.Decision, error) {
	return permission.Decision{Allowed: s.allowed}, s.err
}

// stubLimit answers rate-limit checks with a scripted decision or error.
type stubLimit struct {
	allowed bool
	err     error
}

func (s stubLimit) Allow(context.Context, string, float64) (ratelimit.Decision, error) {
	return ratelimit.Decision{Allowed: s.allowed, RetryAfter: time.Second}, s.err
}
func (stubLimit) Reset(context.Context, string) error { return nil }
func (stubLimit) Close() error                        { return nil }
func (stubLimit) Name() string                        { return "stub" }

// stubSingleLock is a no-op lease.
type stubSingleLock struct{}

func (stubSingleLock) Key() string                                 { return "stub" }
func (stubSingleLock) Extend(context.Context, time.Duration) error { return nil }
func (stubSingleLock) Unlock(context.Context) error                { return nil }

// stubLocker answers TryAcquire with a scripted held/error outcome.
type stubLocker struct {
	held bool
	err  error
}

func (s stubLocker) TryAcquire(context.Context, string, time.Duration) (lock.Lock, bool, error) {
	if s.err != nil {
		return nil, false, s.err
	}
	if s.held {
		return nil, false, nil
	}
	return stubSingleLock{}, true, nil
}
func (stubLocker) Acquire(context.Context, string, time.Duration) (lock.Lock, error) {
	return nil, errBoom
}
func (stubLocker) Close(context.Context) error { return nil }

// stubIdem answers idempotency Begin/Complete with scripted outcomes.
type stubIdem struct {
	beginErr    error
	completeErr error
	replay      bool
	result      []byte
}

func (s stubIdem) Begin(context.Context, string, idempotency.BeginOptions) (idempotency.Outcome, error) {
	if s.beginErr != nil {
		return idempotency.Outcome{}, s.beginErr
	}
	return idempotency.Outcome{Replay: s.replay, Result: s.result}, nil
}
func (s stubIdem) Complete(context.Context, string, []byte, []byte) error {
	return s.completeErr
}
func (stubIdem) Forget(context.Context, string) error { return nil }
func (stubIdem) Close() error                         { return nil }

// stubSessionStore answers session calls with scripted errors.
type stubSessionStore struct {
	createErr error
	saveErr   error
}

func (s stubSessionStore) Create(context.Context, time.Duration) (session.Session, error) {
	if s.createErr != nil {
		return session.Session{}, s.createErr
	}
	return session.Session{ID: "s1", Data: map[string]any{}}, nil
}
func (stubSessionStore) Get(context.Context, string) (session.Session, error) {
	return session.Session{}, errBoom
}
func (s stubSessionStore) Save(context.Context, session.Session) error { return s.saveErr }
func (stubSessionStore) Delete(context.Context, string) error          { return nil }
func (stubSessionStore) Close() error                                  { return nil }

// stubQueueStore fails Push on demand.
type stubQueueStore struct{ pushErr error }

func (s stubQueueStore) Push(context.Context, string, queue.Payload, queue.Headers) error {
	return s.pushErr
}
func (stubQueueStore) PushDelayed(context.Context, string, queue.Payload, queue.Headers, time.Duration) error {
	return nil
}
func (stubQueueStore) Pop(context.Context, string) (queue.Message, error) {
	return queue.Message{}, queue.ErrEmpty
}
func (stubQueueStore) Ack(context.Context, queue.Message) error        { return nil }
func (stubQueueStore) Nack(context.Context, queue.Message, bool) error { return nil }
func (stubQueueStore) Length(context.Context, string) (int64, error)   { return 0, nil }
func (stubQueueStore) IsEmpty(context.Context, string) (bool, error)   { return true, nil }
func (stubQueueStore) Close() error                                    { return nil }
func (stubQueueStore) Name() string                                    { return "stub" }

// stubEventBus fails Publish on demand.
type stubEventBus struct{ pubErr error }

func (s stubEventBus) Publish(context.Context, string, eventbus.Payload, eventbus.Headers) error {
	return s.pubErr
}
func (stubEventBus) Subscribe(context.Context, string, eventbus.Handler) (func(), error) {
	return func() {}, nil
}
func (stubEventBus) SubscribeChan(context.Context, string, int) (<-chan eventbus.Message, error) {
	return nil, errBoom
}
func (stubEventBus) Unsubscribe(string, <-chan eventbus.Message) error { return nil }
func (stubEventBus) Close() error                                      { return nil }
func (stubEventBus) Name() string                                      { return "stub" }

// stubSearchStore fails Index/Search on demand.
type stubSearchStore struct {
	indexErr  error
	searchErr error
}

func (s stubSearchStore) Index(context.Context, search.Document) error      { return s.indexErr }
func (stubSearchStore) IndexBatch(context.Context, []search.Document) error { return nil }
func (stubSearchStore) Delete(context.Context, string) error                { return nil }
func (s stubSearchStore) Search(context.Context, string, search.QueryOptions) (search.Result, error) {
	return search.Result{}, s.searchErr
}
func (stubSearchStore) Close() error { return nil }

// stubVectorStore fails Upsert/Query on demand.
type stubVectorStore struct {
	upsertErr error
	queryErr  error
}

func (s stubVectorStore) Upsert(context.Context, vectorstore.Vector) error { return s.upsertErr }
func (stubVectorStore) UpsertBatch(context.Context, []vectorstore.Vector) error {
	return nil
}
func (stubVectorStore) Delete(context.Context, string) error { return nil }
func (s stubVectorStore) Query(context.Context, []float32, int) ([]vectorstore.ScoreMatch, error) {
	return nil, s.queryErr
}
func (stubVectorStore) Close() error { return nil }

// stubStorage fails PresignUpload on demand.
type stubStorage struct{ presignErr error }

func (s stubStorage) PresignUpload(context.Context, string, string, string, time.Duration) (storage.PresignedURL, error) {
	if s.presignErr != nil {
		return storage.PresignedURL{}, s.presignErr
	}
	return storage.PresignedURL{URL: "https://example.com/upload"}, nil
}
func (stubStorage) PresignDownload(context.Context, string, string, time.Duration) (storage.PresignedURL, error) {
	return storage.PresignedURL{}, nil
}
func (stubStorage) Exists(context.Context, string, string) (bool, error) { return false, nil }
func (stubStorage) Delete(context.Context, string, string) error         { return nil }
func (stubStorage) Move(context.Context, string, string, string, string) error {
	return nil
}
func (stubStorage) Close(context.Context) error { return nil }
func (stubStorage) Name() string                { return "stub" }

// stubMedia fails Upload on demand.
type stubMedia struct{ uploadErr error }

func (s stubMedia) Upload(context.Context, string, []byte, media.UploadOptions) (media.Asset, error) {
	if s.uploadErr != nil {
		return media.Asset{}, s.uploadErr
	}
	return media.Asset{ID: "a1"}, nil
}
func (stubMedia) Download(context.Context, string) ([]byte, error) { return nil, nil }
func (stubMedia) DownloadRange(context.Context, string, int64, int64) ([]byte, error) {
	return nil, nil
}
func (stubMedia) Delete(context.Context, string) error               { return nil }
func (stubMedia) Stat(context.Context, string) (media.Info, error)   { return media.Info{}, nil }
func (stubMedia) Probe(context.Context, string) (media.Probe, error) { return media.Probe{}, nil }
func (stubMedia) Transform(context.Context, string, media.TransformOps) (string, error) {
	return "", nil
}
func (stubMedia) Close() error { return nil }

// stubAI answers Generate with scripted content or error.
type stubAI struct {
	content string
	genErr  error
}

func (s stubAI) Generate(context.Context, string, []ai.Message, ai.GenerateOptions) (ai.Generation, error) {
	if s.genErr != nil {
		return ai.Generation{}, s.genErr
	}
	return ai.Generation{Content: s.content}, nil
}
func (stubAI) Stream(context.Context, string, []ai.Message, ai.GenerateOptions) (<-chan ai.StreamChunk, error) {
	return nil, errBoom
}
func (stubAI) Embed(context.Context, string, []string, ai.EmbedOptions) ([][]float32, error) {
	return nil, errBoom
}
func (stubAI) Close() error { return nil }

// stubI18n fails every translation.
type stubI18n struct{}

func (stubI18n) Translate(context.Context, string, string, map[string]string) (string, error) {
	return "", errBoom
}
func (stubI18n) Locales(context.Context) ([]string, error) { return []string{"en"}, nil }
func (stubI18n) Close() error                              { return nil }

// stubCrypto fails Encrypt on demand and decrypts everything else.
type stubCrypto struct{ encErr error }

func (s stubCrypto) Encrypt(context.Context, []byte) ([]byte, error) {
	if s.encErr != nil {
		return nil, s.encErr
	}
	return []byte("ciphertext"), nil
}
func (stubCrypto) Decrypt(context.Context, []byte) ([]byte, error) {
	return []byte("plaintext"), nil
}
func (stubCrypto) Sign(context.Context, []byte) ([]byte, error) { return []byte("sig"), nil }
func (stubCrypto) Verify(context.Context, []byte, []byte) (bool, error) {
	return true, nil
}
func (stubCrypto) Mac(context.Context, []byte) ([]byte, error) { return []byte("mac"), nil }
func (stubCrypto) VerifyMac(context.Context, []byte, []byte) (bool, error) {
	return true, nil
}

// stubNotifier fails Notify on demand.
type stubNotifier struct{ err error }

func (s stubNotifier) Notify(context.Context, *notification.Notification) error { return s.err }
func (stubNotifier) Close() error                                               { return nil }

// stubWebhook fails Register on demand.
type stubWebhook struct{ regErr error }

func (s stubWebhook) Register(context.Context, string, string, string) error {
	return s.regErr
}
func (stubWebhook) Unregister(context.Context, string, string) error { return nil }
func (stubWebhook) Deliver(context.Context, string, []byte) error    { return nil }
func (stubWebhook) Close() error                                     { return nil }

// stubWorkflow fails Start on demand.
type stubWorkflow struct{ startErr error }

func (s stubWorkflow) Start(context.Context, string, any, string) (workflow.RunID, error) {
	if s.startErr != nil {
		return "", s.startErr
	}
	return workflow.RunID("run-1"), nil
}
func (stubWorkflow) Signal(context.Context, workflow.RunID, string, any) error { return nil }
func (stubWorkflow) Query(context.Context, workflow.RunID, string, any) error  { return nil }
func (stubWorkflow) Cancel(context.Context, workflow.RunID) error              { return nil }
func (stubWorkflow) Close() error                                              { return nil }

// stubDocument renders scripted bytes or fails.
type stubDocument struct {
	out []byte
	err error
}

func (s stubDocument) Render(context.Context, []byte, document.OutputFormat) ([]byte, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.out, nil
}
func (stubDocument) Close() error { return nil }

// stubTenant resolves a scripted id or fails.
type stubTenant struct {
	id  string
	err error
}

func (s stubTenant) Resolve(context.Context, map[string]string) (string, error) {
	return s.id, s.err
}
func (s stubTenant) Scoped(ctx context.Context, _ string) (context.Context, error) {
	return ctx, nil
}
func (stubTenant) Close() error { return nil }

// stubSpan is a no-op span.
type stubSpan struct{}

func (stubSpan) SetAttributes(...observability.Attr) {}
func (stubSpan) RecordError(error)                   {}
func (stubSpan) End()                                {}

// stubTracer returns no-op spans.
type stubTracer struct{}

func (stubTracer) Start(ctx context.Context, _ string) (context.Context, observability.Span) {
	return ctx, stubSpan{}
}
func (stubTracer) Shutdown(context.Context) error { return nil }

// stubMetrics fails Counter on demand.
type stubMetrics struct{ counterErr error }

func (s stubMetrics) Counter(context.Context, string, float64, ...observability.Attr) error {
	return s.counterErr
}
func (stubMetrics) Gauge(context.Context, string, float64, ...observability.Attr) error {
	return nil
}
func (stubMetrics) Histogram(context.Context, string, float64, ...observability.Attr) error {
	return nil
}
func (stubMetrics) Shutdown(context.Context) error { return nil }

// stubProvider serves the scripted meter and no-op tracer.
type stubProvider struct{ metrics stubMetrics }

func (stubProvider) Tracer(string) observability.Tracer   { return stubTracer{} }
func (s stubProvider) Meter(string) observability.Metrics { return s.metrics }
func (stubProvider) Shutdown(context.Context) error       { return nil }

// stubAuth verifies as a scripted subject and fails Issue on demand.
type stubAuth struct {
	subject  string
	issueErr error
}

func (s stubAuth) Verify(context.Context, string) (auth.Claims, error) {
	return auth.Claims{Subject: s.subject}, nil
}
func (s stubAuth) Issue(context.Context, string, map[string]any, time.Duration) (auth.Token, error) {
	if s.issueErr != nil {
		return auth.Token{}, s.issueErr
	}
	return auth.Token{Value: "tok"}, nil
}
func (stubAuth) Revoke(context.Context, string) error { return nil }
func (stubAuth) Close() error                         { return nil }

// stubPassword fails Hash on demand and verifies everything else.
type stubPassword struct{ hashErr error }

func (s stubPassword) Hash(context.Context, string) (string, error) {
	if s.hashErr != nil {
		return "", s.hashErr
	}
	return "hash", nil
}
func (stubPassword) Verify(context.Context, string, string) (bool, error) {
	return true, nil
}
func (stubPassword) NeedsRehash(context.Context, string) (bool, error) { return false, nil }

// newBareAPI routes a server with exactly the given deps. Handlers resolve
// batteries best-effort, so test doubles slot in without a container.
func newBareAPI(t *testing.T, deps api.Deps) http.Handler {
	t.Helper()

	routerstdhttp.Register()
	c := container.New(config.Default())
	t.Cleanup(func() { _ = c.Close(t.Context()) })
	r, err := c.Router()
	if err != nil {
		t.Fatalf("Router: %v", err)
	}
	api.New(deps).Routes(r)
	return r
}

// TestDemoNilBatteries pins 501 for every demo route when its battery is
// unconfigured. The server stays up; nothing panics on nil services.
func TestDemoNilBatteries(t *testing.T) {
	h := newBareAPI(t, api.Deps{})

	jsonBody := map[string]map[string]any{
		"/demo/permission/check": {"subject": "u", "role": "member", "resource": "product", "action": "product.read"},
		"/demo/idempotency/k":    {"data": "x"},
		"/demo/session":          {"user": "ann"},
		"/demo/eventbus/publish": {"topic": "t", "payload": "p"},
		"/demo/search/index":     {"id": "d", "content": "c"},
		"/demo/vector/upsert":    {"id": "v"},
		"/demo/storage/presign":  {},
		"/demo/ai/generate":      {},
		"/demo/geo/geocode":      {},
		"/demo/crypto/encrypt":   {"plaintext": "x"},
		"/demo/crypto/decrypt":   {"ciphertext": "eA=="},
		"/demo/notification":     {},
		"/demo/webhook/register": {},
		"/demo/workflow/start":   {},
		"/demo/document/render":  {},
		"/demo/vector/query":     {},
	}

	tests := []struct{ method, path string }{
		{"GET", "/demo/cache/k"},
		{"POST", "/demo/cache/k"},
		{"GET", "/demo/flag/k"},
		{"POST", "/demo/permission/check"},
		{"POST", "/demo/ratelimit/k"},
		{"POST", "/demo/lock/k"},
		{"POST", "/demo/idempotency/k"},
		{"POST", "/demo/session"},
		{"GET", "/demo/session/missing"},
		{"POST", "/demo/queue/t"},
		{"POST", "/demo/eventbus/publish"},
		{"POST", "/demo/search/index"},
		{"GET", "/demo/search?q=x"},
		{"POST", "/demo/vector/upsert"},
		{"POST", "/demo/vector/query"},
		{"POST", "/demo/storage/presign"},
		{"POST", "/demo/media/upload"},
		{"POST", "/demo/ai/generate"},
		{"POST", "/demo/geo/geocode"},
		{"GET", "/demo/i18n/en/hello"},
		{"POST", "/demo/crypto/encrypt"},
		{"POST", "/demo/crypto/decrypt"},
		{"GET", "/demo/secrets/FOO"},
		{"POST", "/demo/notification"},
		{"POST", "/demo/webhook/register"},
		{"POST", "/demo/workflow/start"},
		{"POST", "/demo/document/render"},
		{"GET", "/demo/tenant"},
		{"GET", "/demo/observability"},
		{"POST", "/demo/jobs/dispatch/nope"},
	}
	for _, tc := range tests {
		var body any
		if b, ok := jsonBody[tc.path]; ok {
			body = b
		} else if tc.method == "POST" && strings.HasPrefix(tc.path, "/demo/cache/") {
			body = "world"
		}
		rec := do(t, h, tc.method, tc.path, "", body)
		if rec.Code != http.StatusNotImplemented {
			t.Errorf("%s %s = %d, want 501 body %s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "not configured") {
			t.Errorf("%s %s body = %s, want not configured", tc.method, tc.path, rec.Body.String())
		}
	}
}

// TestDemoMalformedJSON pins 400 for invalid JSON on every demo endpoint
// that decodes a body.
func TestDemoMalformedJSON(t *testing.T) {
	s := newTestSetup(t)
	h := s.handler

	for _, path := range []string{
		"/demo/permission/check",
		"/demo/idempotency/k",
		"/demo/session",
		"/demo/eventbus/publish",
		"/demo/search/index",
		"/demo/vector/upsert",
		"/demo/vector/query",
		"/demo/storage/presign",
		"/demo/ai/generate",
		"/demo/geo/geocode",
		"/demo/crypto/encrypt",
		"/demo/crypto/decrypt",
		"/demo/notification",
		"/demo/webhook/register",
		"/demo/workflow/start",
		"/demo/document/render",
	} {
		rec := doRaw(t, h, "POST", path, "", "{bad-json")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("POST %s = %d, want 400 body %s", path, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "invalid JSON") {
			t.Errorf("POST %s body = %s, want invalid JSON", path, rec.Body.String())
		}
	}
}

// TestDemoBatteryErrors pins the 5xx contracts when backends fail, using
// one stub battery per handler so no real infrastructure is needed.
func TestDemoBatteryErrors(t *testing.T) {
	validJSON := map[string]any{"data": "x"}

	tests := []struct {
		name   string
		deps   api.Deps
		method string
		path   string
		body   any
		want   int
		msg    string
	}{
		{"cache get error", api.Deps{Cache: stubCache{getErr: errBoom}}, "GET", "/demo/cache/k", nil, 500, "cache error"},
		{"cache set error", api.Deps{Cache: stubCache{setErr: errBoom}}, "POST", "/demo/cache/k", "world", 500, "cache error"},
		{"flag all fail", api.Deps{Flag: stubFlag{boolErr: errBoom, strErr: errBoom}}, "GET", "/demo/flag/k", nil, 404, "flag not found"},
		{"flag string fallback", api.Deps{Flag: stubFlag{boolErr: errBoom, strVal: "on"}}, "GET", "/demo/flag/k", nil, 200, "string"},
		{"permission error", api.Deps{Permission: stubPerm{err: errBoom}}, "POST", "/demo/permission/check",
			map[string]string{"subject": "u", "role": "m", "resource": "p", "action": "a"}, 500, "permission error"},
		{"ratelimit error", api.Deps{RateLimit: stubLimit{err: errBoom}}, "POST", "/demo/ratelimit/k", nil, 500, "ratelimit error"},
		{"ratelimit deny", api.Deps{RateLimit: stubLimit{}}, "POST", "/demo/ratelimit/k", nil, 200, "allowed"},
		{"lock error", api.Deps{Lock: stubLocker{err: errBoom}}, "POST", "/demo/lock/k", nil, 500, "lock error"},
		{"lock conflict", api.Deps{Lock: stubLocker{held: true}}, "POST", "/demo/lock/k", nil, 409, "acquired"},
		{"idempotency begin error", api.Deps{Idempotency: stubIdem{beginErr: errBoom}}, "POST", "/demo/idempotency/k", validJSON, 500, "idempotency error"},
		{"idempotency complete error", api.Deps{Idempotency: stubIdem{completeErr: errBoom}}, "POST", "/demo/idempotency/k", validJSON, 500, "idempotency error"},
		{"session create error", api.Deps{Session: stubSessionStore{createErr: errBoom}}, "POST", "/demo/session", validJSON, 500, "session error"},
		{"session save error", api.Deps{Session: stubSessionStore{saveErr: errBoom}}, "POST", "/demo/session", validJSON, 500, "session error"},
		{"session get error", api.Deps{Session: stubSessionStore{}}, "GET", "/demo/session/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", nil, 404, "not found"},
		{"queue push error", api.Deps{Queue: stubQueueStore{pushErr: errBoom}}, "POST", "/demo/queue/t", "hello", 500, "queue error"},
		{"eventbus publish error", api.Deps{EventBus: stubEventBus{pubErr: errBoom}}, "POST", "/demo/eventbus/publish",
			map[string]string{"topic": "t", "payload": "p"}, 500, "eventbus error"},
		{"search index error", api.Deps{Search: stubSearchStore{indexErr: errBoom}}, "POST", "/demo/search/index",
			map[string]string{"id": "d", "content": "c"}, 500, "search error"},
		{"search query error", api.Deps{Search: stubSearchStore{searchErr: errBoom}}, "GET", "/demo/search?q=x", nil, 500, "search error"},
		{"vector upsert error", api.Deps{VectorStore: stubVectorStore{upsertErr: errBoom}}, "POST", "/demo/vector/upsert",
			map[string]string{"id": "v"}, 500, "vectorstore error"},
		{"vector query error", api.Deps{VectorStore: stubVectorStore{queryErr: errBoom}}, "POST", "/demo/vector/query",
			map[string]any{}, 500, "vectorstore error"},
		{"storage presign error", api.Deps{Storage: stubStorage{presignErr: errBoom}}, "POST", "/demo/storage/presign",
			map[string]any{}, 500, "storage error"},
		{"media upload error", api.Deps{Media: stubMedia{uploadErr: errBoom}}, "POST", "/demo/media/upload", "bytes", 500, "media error"},
		{"ai success", api.Deps{AI: stubAI{content: "hi"}}, "POST", "/demo/ai/generate",
			map[string]string{"prompt": "hello", "model": "m"}, 200, "hi"},
		{"ai defaults", api.Deps{AI: stubAI{content: "hi"}}, "POST", "/demo/ai/generate", map[string]any{}, 200, "hi"},
		{"ai error", api.Deps{AI: stubAI{genErr: errBoom}}, "POST", "/demo/ai/generate", map[string]any{}, 502, "ai error"},
		{"i18n error", api.Deps{I18n: stubI18n{}}, "GET", "/demo/i18n/en/hello", nil, 404, "no message"},
		{"crypto encrypt error", api.Deps{Crypto: stubCrypto{encErr: errBoom}}, "POST", "/demo/crypto/encrypt",
			map[string]string{"plaintext": "x"}, 500, "crypto error"},
		{"notification error", api.Deps{Notification: stubNotifier{err: errBoom}}, "POST", "/demo/notification",
			map[string]string{"target": "t", "title": "t", "body": "b"}, 500, "notification error"},
		{"webhook error", api.Deps{Webhook: stubWebhook{regErr: errBoom}}, "POST", "/demo/webhook/register",
			map[string]string{"event": "e", "target": "https://example.com/h"}, 500, "webhook error"},
		{"workflow error", api.Deps{Workflow: stubWorkflow{startErr: errBoom}}, "POST", "/demo/workflow/start",
			map[string]string{"name": "w"}, 500, "workflow error"},
		{"document success", api.Deps{Document: stubDocument{out: []byte("%PDF")}}, "POST", "/demo/document/render",
			map[string]string{"format": "pdf", "source": "<h1>x</h1>"}, 200, "bytes"},
		{"document defaults", api.Deps{Document: stubDocument{out: []byte("%PDF")}}, "POST", "/demo/document/render",
			map[string]any{}, 200, "bytes"},
		{"document error", api.Deps{Document: stubDocument{err: errBoom}}, "POST", "/demo/document/render",
			map[string]any{}, 502, "document error"},
		{"tenant error", api.Deps{Tenant: stubTenant{err: errBoom}}, "GET", "/demo/tenant", nil, 500, "tenant error"},
		{"observability error", api.Deps{Observability: stubProvider{metrics: stubMetrics{counterErr: errBoom}}},
			"GET", "/demo/observability", nil, 500, "observability error"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newBareAPI(t, tc.deps)
			rec := do(t, h, tc.method, tc.path, "", tc.body)
			if rec.Code != tc.want {
				t.Fatalf("%s %s = %d, want %d body %s", tc.method, tc.path, rec.Code, tc.want, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tc.msg) {
				t.Fatalf("body = %s, want %q", rec.Body.String(), tc.msg)
			}
		})
	}
}

// TestDemoDefaults pins the zero-value defaults: empty vector/top_k,
// empty bucket/key, empty body/geo/notification/webhook/workflow fields,
// search without id, and cache TTL parsing.
func TestDemoDefaults(t *testing.T) {
	s := newTestSetup(t)
	h := s.handler

	rec := do(t, h, "POST", "/demo/search/index", "", map[string]string{"content": "hello world"})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "indexed") {
		t.Fatalf("search default id = %d %s", rec.Code, rec.Body.String())
	}
	rec = do(t, h, "POST", "/demo/vector/upsert", "", map[string]any{})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "upserted") {
		t.Fatalf("vector default upsert = %d %s", rec.Code, rec.Body.String())
	}
	rec = do(t, h, "POST", "/demo/vector/query", "", map[string]any{})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "matches") {
		t.Fatalf("vector default query = %d %s", rec.Code, rec.Body.String())
	}
	rec = do(t, h, "POST", "/demo/storage/presign", "", map[string]any{})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"bucket":"demo"`) {
		t.Fatalf("storage defaults = %d %s", rec.Code, rec.Body.String())
	}
	rec = do(t, h, "POST", "/demo/media/upload", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("media default body = %d %s", rec.Code, rec.Body.String())
	}
	rec = do(t, h, "POST", "/demo/geo/geocode", "", map[string]any{})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "results") {
		t.Fatalf("geo default address = %d %s", rec.Code, rec.Body.String())
	}
	rec = do(t, h, "POST", "/demo/notification", "", map[string]any{})
	if rec.Code != http.StatusOK {
		t.Fatalf("notification default target = %d %s", rec.Code, rec.Body.String())
	}
	rec = do(t, h, "POST", "/demo/webhook/register", "", map[string]string{"target": "https://example.com/hook", "secret": "s"})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "demo.event") {
		t.Fatalf("webhook default event = %d %s", rec.Code, rec.Body.String())
	}
	rec = do(t, h, "POST", "/demo/webhook/register", "", map[string]string{"secret": "s"})
	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Fatalf("webhook default target = %d, want 200/500", rec.Code)
	}
	rec = do(t, h, "POST", "/demo/workflow/start", "", map[string]any{})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "run_id") {
		t.Fatalf("workflow default name = %d %s", rec.Code, rec.Body.String())
	}
	rec = do(t, h, "POST", "/demo/cache/ttl-key?ttl=1m", "", "v")
	if rec.Code != http.StatusOK {
		t.Fatalf("cache set ttl = %d %s", rec.Code, rec.Body.String())
	}
	rec = do(t, h, "GET", "/demo/cache/ttl-key", "", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"value":"v"`) {
		t.Fatalf("cache get ttl key = %d %s", rec.Code, rec.Body.String())
	}
	rec = do(t, h, "GET", "/demo/i18n/en/no-such-key-xyz", "", nil)
	if rec.Code != http.StatusOK && rec.Code != http.StatusNotFound {
		t.Fatalf("i18n unknown key = %d, want 200/404", rec.Code)
	}
}

// TestDemoDBErrors pins 4xx/5xx (never panics) for every CRUD route when the
// database fails. Auth is stubbed so the DB fault is isolated.
func TestDemoDBErrors(t *testing.T) {
	h := newBareAPI(t, api.Deps{DB: stubFailDB{}, Auth: stubAuth{subject: "u1"}, Password: stubPassword{}})

	tests := []struct {
		method, path string
		body         any
		token        string
		want         int
		msg          string
	}{
		{"POST", "/register", map[string]string{"email": "a@b.c", "name": "N", "password": "password123"}, "", 500, "lookup failed"},
		{"POST", "/login", map[string]string{"email": "a@b.c", "password": "password123"}, "", 401, "invalid credentials"},
		{"GET", "/me", nil, "tok", 500, "lookup failed"},
		{"POST", "/v1/products", map[string]any{"name": "W"}, "tok", 400, "category_id required"},
		{"GET", "/v1/products/p-1", nil, "", 500, "db error"},
		{"GET", "/v1/products", nil, "", 500, "db error"},
		{"POST", "/v1/orders", map[string]any{"product_ids": []string{"p-1"}}, "tok", 400, "unknown product"},
		{"GET", "/v1/orders", nil, "tok", 500, "db error"},
		{"GET", "/v1/orders/o-1", nil, "tok", 500, "db error"},
		{"POST", "/v1/posts", map[string]string{"title": "T", "body": "b"}, "tok", 500, "could not create"},
		{"GET", "/v1/posts", nil, "", 500, "db error"},
	}
	for _, tc := range tests {
		rec := do(t, h, tc.method, tc.path, tc.token, tc.body)
		if rec.Code != tc.want {
			t.Errorf("%s %s = %d, want %d body %s", tc.method, tc.path, rec.Code, tc.want, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), tc.msg) {
			t.Errorf("%s %s body = %s, want %q", tc.method, tc.path, rec.Body.String(), tc.msg)
		}
	}
}

// TestDemoAuthEdge pins auth-adjacent error contracts: bad credentials,
// issue/hash failures, empty subjects, missing users, and deny decisions.
func TestDemoAuthEdge(t *testing.T) {
	s := newTestSetup(t)
	h := s.handler

	register(t, h, "auth@example.com", "Auth")
	rec := do(t, h, "POST", "/login", "", map[string]string{"email": "auth@example.com", "password": "wrongpass1"})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("wrong password = %d, want 401", rec.Code)
	}
	rec = do(t, h, "POST", "/login", "", map[string]string{"email": "nobody@example.com", "password": "password123"})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("unknown email = %d, want 401", rec.Code)
	}
	token := login(t, h, "auth@example.com")

	hIssue := newBareAPI(t, api.Deps{DB: s.db, Auth: stubAuth{subject: "u1", issueErr: errBoom}, Password: stubPassword{}})
	rec = do(t, hIssue, "POST", "/login", "", map[string]string{"email": "auth@example.com", "password": "password123"})
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "issue failed") {
		t.Errorf("issue error = %d %s, want 500 issue failed", rec.Code, rec.Body.String())
	}

	hHash := newBareAPI(t, api.Deps{DB: s.db, Auth: stubAuth{subject: "u1"}, Password: stubPassword{hashErr: errBoom}})
	rec = do(t, hHash, "POST", "/register", "", map[string]string{"email": "new@example.com", "name": "New", "password": "password123"})
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "hash failed") {
		t.Errorf("hash error = %d %s, want 500 hash failed", rec.Code, rec.Body.String())
	}

	hDeny := newBareAPI(t, api.Deps{DB: s.db, Auth: stubAuth{subject: "u1"}, Password: stubPassword{}, RateLimit: stubLimit{}})
	rec = do(t, hDeny, "POST", "/v1/products", "tok", map[string]any{"name": "Widget", "category_id": "cat-x"})
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("ratelimit deny = %d, want 429 body %s", rec.Code, rec.Body.String())
	}

	if _, err := s.db.Exec(t.Context(), `DELETE FROM users`); err != nil {
		t.Fatalf("delete users: %v", err)
	}
	if rec := do(t, h, "GET", "/me", token, nil); rec.Code != http.StatusNotFound {
		t.Errorf("me after delete = %d, want 404 body %s", rec.Code, rec.Body.String())
	}

	hEmpty := newBareAPI(t, api.Deps{Auth: stubAuth{}})
	if rec := do(t, hEmpty, "GET", "/me", "tok", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("empty subject = %d, want 401", rec.Code)
	}
}

// TestDemoMinimalDeps runs the CRUD flow with only required deps set,
// covering every best-effort `!= nil` false branch.
func TestDemoMinimalDeps(t *testing.T) {
	full := newTestSetup(t)

	authjwt.Register()
	passwordargon2.Register()
	logslog.Register()
	routerstdhttp.Register()

	cfg := config.Default()
	cfg.Auth.Options.JWT.Secret = testJWTSecret
	c := container.New(cfg)
	t.Cleanup(func() { _ = c.Close(t.Context()) })
	authInst, err := c.Auth()
	if err != nil {
		t.Fatalf("Auth: %v", err)
	}
	hasher, err := c.Password()
	if err != nil {
		t.Fatalf("Password: %v", err)
	}
	logger, err := c.Log()
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	r, err := c.Router()
	if err != nil {
		t.Fatalf("Router: %v", err)
	}
	api.New(api.Deps{DB: full.db, Auth: authInst, Password: hasher, Log: logger}).Routes(r)
	h := r

	register(t, h, "min@example.com", "Min")
	token := login(t, h, "min@example.com")

	if rec := do(t, h, "GET", "/me", token, nil); rec.Code != http.StatusOK {
		t.Fatalf("me: %d %s", rec.Code, rec.Body.String())
	}
	makeCategory(t, full, "cat-min", "Gadgets")
	pid := makeProduct(t, h, token, "cat-min", "Widget", 2500)

	if rec := do(t, h, "GET", "/v1/products/"+pid, "", nil); rec.Code != http.StatusOK {
		t.Fatalf("get product: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(t, h, "GET", "/v1/products", "", nil); rec.Code != http.StatusOK {
		t.Fatalf("list products: %d %s", rec.Code, rec.Body.String())
	}
	rec := do(t, h, "POST", "/v1/orders", token, map[string]any{"product_ids": []string{pid}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create order: %d %s", rec.Code, rec.Body.String())
	}
	if got := do(t, h, "GET", "/v1/orders", token, nil); got.Code != http.StatusOK {
		t.Fatalf("list orders: %d %s", got.Code, got.Body.String())
	}
	if got := do(t, h, "POST", "/v1/posts", token, map[string]string{"title": "Hi", "body": "x"}); got.Code != http.StatusCreated {
		t.Fatalf("create post: %d %s", got.Code, got.Body.String())
	}
	if rec.Header().Get("X-Crypto") != "" {
		t.Fatalf("create post: want no X-Crypto header without crypto battery")
	}
	if rec := do(t, h, "GET", "/v1/posts?user_id=someone", "", nil); rec.Code != http.StatusOK {
		t.Fatalf("list posts filtered: %d %s", rec.Code, rec.Body.String())
	}
}

// TestRegisterDemoWorkflowNil pins the no-op path when the engine does not
// support host-registered steps.
func TestRegisterDemoWorkflowNil(t *testing.T) {
	t.Helper()
	api.RegisterDemoWorkflow(nil)
	api.RegisterDemoWorkflow(stubWorkflow{})
}
