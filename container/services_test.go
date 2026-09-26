package container

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/core/ai"
	"github.com/zenta-dev/zever/core/analytics"
	"github.com/zenta-dev/zever/core/auth"
	"github.com/zenta-dev/zever/core/billing"
	"github.com/zenta-dev/zever/core/cache"
	"github.com/zenta-dev/zever/core/crypto"
	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/document"
	"github.com/zenta-dev/zever/core/eventbus"
	"github.com/zenta-dev/zever/core/flag"
	"github.com/zenta-dev/zever/core/geo"
	"github.com/zenta-dev/zever/core/i18n"
	"github.com/zenta-dev/zever/core/idempotency"
	"github.com/zenta-dev/zever/core/lock"
	"github.com/zenta-dev/zever/core/log"
	"github.com/zenta-dev/zever/core/mailer"
	"github.com/zenta-dev/zever/core/media"
	"github.com/zenta-dev/zever/core/notification"
	"github.com/zenta-dev/zever/core/observability"
	"github.com/zenta-dev/zever/core/password"
	"github.com/zenta-dev/zever/core/payment"
	"github.com/zenta-dev/zever/core/permission"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/core/ratelimit"
	"github.com/zenta-dev/zever/core/router"
	"github.com/zenta-dev/zever/core/scheduler"
	"github.com/zenta-dev/zever/core/search"
	"github.com/zenta-dev/zever/core/secrets"
	"github.com/zenta-dev/zever/core/session"
	"github.com/zenta-dev/zever/core/storage"
	"github.com/zenta-dev/zever/core/tenant"
	"github.com/zenta-dev/zever/core/vectorstore"
	"github.com/zenta-dev/zever/core/webhook"
	"github.com/zenta-dev/zever/core/workflow"
)

// testConfig returns Default with every default adapter name wired to a
// zero-value fake below, so no API keys, secrets, DB paths, or fixture files
// are needed. The container no longer wires any adapter itself: resolution
// requires caller-side registration, and tests stand in for the caller here.
// All fixtures are local and deterministic; nothing touches the network.
func testConfig(t *testing.T) *config.Config {
	t.Helper()

	registerFakeAdapters()

	return config.Default()
}

// registerFakeAdapters wires zero-value fakes under the default adapter
// names of every service, so container tests import zero nested modules.
// Registries are global; duplicate errors from repeated or cross-file
// registration are ignored — first registration wins and every fake here
// satisfies the same battery interface.
func registerFakeAdapters() {
	_ = ai.Register(ai.Anthropic, newFakeAI)
	_ = analytics.Register(analytics.Log, newFakeAnalytics)
	_ = auth.Register(auth.JWT, newFakeAuth)
	_ = billing.Register(billing.Stub, newFakeBilling)
	_ = cache.Register(cache.Memory, newFakeCache)
	_ = crypto.Register(crypto.AdapterLocal, newFakeCrypto)
	_ = db.Register(db.SQLite, newFakeDB)
	_ = document.Register(document.Local, newFakeDocument)
	_ = eventbus.Register(eventbus.Memory, newFakeEventBus)
	_ = flag.Register(flag.Static, newFakeFlag)
	_ = geo.Register(geo.Static, newFakeGeo)
	_ = i18n.Register(i18n.Embed, newFakeI18n)
	_ = idempotency.Register(idempotency.Memory, newFakeIdempotency)
	_ = lock.Register(lock.Memory, newFakeLock)
	_ = log.Register(log.Noop, newFakeLog)
	_ = log.Register(log.Slog, newFakeLog)
	_ = log.Register(log.Pretty, newFakeLog)
	_ = mailer.Register(mailer.Log, newFakeMailer)
	_ = media.Register(media.Local, newFakeMedia)
	_ = notification.Register(notification.Log, newFakeNotification)
	_ = observability.Register(observability.Noop, newFakeObservability)
	_ = observability.Register(observability.Stdout, newFakeObservability)
	_ = password.Register(password.AdapterArgon2ID, newFakePassword)
	_ = payment.Register(payment.Stub, newFakePayment)
	_ = permission.Register(permission.Noop, newFakePermission)
	_ = queue.Register(queue.Memory, newFakeQueue)
	_ = ratelimit.Register(ratelimit.Memory, newFakeRateLimit)
	_ = router.Register(router.AdapterStdHTTP, newFakeRouter)
	_ = scheduler.Register(scheduler.Embedded, newFakeScheduler)
	_ = search.Register(search.SQLite, newFakeSearch)
	_ = secrets.Register(secrets.Env, newFakeSecrets)
	_ = session.Register(session.Memory, newFakeSession)
	_ = storage.Register(storage.AdapterLocal, newFakeStorage)
	_ = tenant.Register(tenant.Single, newFakeTenant)
	_ = vectorstore.Register(vectorstore.SQLite, newFakeVectorStore)
	_ = webhook.Register(webhook.AdapterHTTP, newFakeWebhook)
	_ = workflow.Register(workflow.Memory, newFakeWorkflow)
}

func newFakeAI(_ ai.Options) (ai.AI, error) { return &fakeAI{}, nil }

// fakeAI implements ai.AI with zero values.
type fakeAI struct{}

func (f *fakeAI) Generate(_ context.Context, _ string, _ []ai.Message, _ ai.GenerateOptions) (ai.Generation, error) {
	return ai.Generation{}, nil
}
func (f *fakeAI) Stream(_ context.Context, _ string, _ []ai.Message, _ ai.GenerateOptions) (<-chan ai.StreamChunk, error) {
	ch := make(chan ai.StreamChunk)
	close(ch)
	return ch, nil
}
func (f *fakeAI) Embed(_ context.Context, _ string, _ []string, _ ai.EmbedOptions) ([][]float32, error) {
	return nil, nil
}
func (f *fakeAI) Close() error { return nil }

func newFakeAuth(_ auth.Options) (auth.Auth, error) { return &fakeAuth{}, nil }

// fakeAuth implements auth.Auth with zero values.
type fakeAuth struct{}

func (f *fakeAuth) Issue(_ context.Context, _ string, _ map[string]any, _ time.Duration) (auth.Token, error) {
	return auth.Token{}, nil
}
func (f *fakeAuth) Verify(_ context.Context, _ string) (auth.Claims, error) {
	return auth.Claims{}, nil
}
func (f *fakeAuth) Revoke(_ context.Context, _ string) error { return nil }
func (f *fakeAuth) Close() error                             { return nil }

func newFakeBilling(_ billing.Options) (billing.Billing, error) { return &fakeBilling{}, nil }

// fakeBilling implements billing.Billing with zero values.
type fakeBilling struct{}

func (f *fakeBilling) CreateCustomer(_ context.Context, _, _, _ string) (billing.Customer, error) {
	return billing.Customer{}, nil
}
func (f *fakeBilling) CreateSubscription(_ context.Context, _, _, _ string) (billing.Subscription, error) {
	return billing.Subscription{}, nil
}
func (f *fakeBilling) CancelSubscription(_ context.Context, _ string) error { return nil }
func (f *fakeBilling) GetInvoice(_ context.Context, _ string) (billing.Invoice, error) {
	return billing.Invoice{}, nil
}
func (f *fakeBilling) Close() error { return nil }

func newFakeDB(_ db.Options) (db.DB, error) { return &fakeDB{}, nil }

// fakeDB implements db.DB plus db.Transactor with zero values.
type fakeDB struct{}

func (f *fakeDB) Query(_ context.Context, _ string, _ ...any) (db.Rows, error) {
	return &fakeRows{}, nil
}
func (f *fakeDB) Exec(_ context.Context, _ string, _ ...any) (int64, error) {
	return 0, nil
}
func (f *fakeDB) Ping(_ context.Context) error  { return nil }
func (f *fakeDB) Close(_ context.Context) error { return nil }
func (f *fakeDB) Dialect() string               { return "sqlite" }
func (f *fakeDB) BeginTx(_ context.Context, _ *db.TxOptions) (db.Tx, error) {
	return &fakeTx{DB: f}, nil
}

// fakeRows implements db.Rows with an empty result set.
type fakeRows struct{}

func (r *fakeRows) Next() bool                 { return false }
func (r *fakeRows) Scan(_ ...any) error        { return nil }
func (r *fakeRows) Close() error               { return nil }
func (r *fakeRows) Columns() ([]string, error) { return nil, nil }
func (r *fakeRows) Err() error                 { return nil }

// fakeTx implements db.Tx as a no-op over fakeDB.
type fakeTx struct{ db.DB }

func (t *fakeTx) Commit(_ context.Context) error               { return nil }
func (t *fakeTx) Rollback(_ context.Context) error             { return nil }
func (t *fakeTx) Savepoint(_ context.Context, _ string) error  { return nil }
func (t *fakeTx) RollbackTo(_ context.Context, _ string) error { return nil }

func newFakeDocument(_ document.Options) (document.Document, error) {
	return &fakeDocument{}, nil
}

// fakeDocument implements document.Document with zero values.
type fakeDocument struct{}

func (f *fakeDocument) Render(_ context.Context, _ []byte, _ document.OutputFormat) ([]byte, error) {
	return nil, nil
}
func (f *fakeDocument) Close() error { return nil }

func newFakeMedia(_ media.Options) (media.Media, error) { return &fakeMedia{}, nil }

// fakeMedia implements media.Media with zero values.
type fakeMedia struct{}

func (f *fakeMedia) Upload(_ context.Context, _ string, _ []byte, _ media.UploadOptions) (media.Asset, error) {
	return media.Asset{}, nil
}
func (f *fakeMedia) Download(_ context.Context, _ string) ([]byte, error) { return nil, nil }
func (f *fakeMedia) DownloadRange(_ context.Context, _ string, _, _ int64) ([]byte, error) {
	return nil, nil
}
func (f *fakeMedia) Delete(_ context.Context, _ string) error { return nil }
func (f *fakeMedia) Stat(_ context.Context, _ string) (media.Info, error) {
	return media.Info{}, nil
}
func (f *fakeMedia) Probe(_ context.Context, _ string) (media.Probe, error) {
	return media.Probe{}, nil
}
func (f *fakeMedia) Transform(_ context.Context, _ string, _ media.TransformOps) (string, error) {
	return "", nil
}
func (f *fakeMedia) Close() error { return nil }

func newFakePassword(_ password.Options) (password.Hasher, error) {
	return &fakePassword{}, nil
}

// fakePassword implements password.Hasher with zero values.
type fakePassword struct{}

func (f *fakePassword) Hash(_ context.Context, _ string) (string, error) { return "", nil }
func (f *fakePassword) Verify(_ context.Context, _, _ string) (bool, error) {
	return false, nil
}
func (f *fakePassword) NeedsRehash(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func newFakeScheduler(_ scheduler.Options) (scheduler.Scheduler, error) {
	return &fakeScheduler{}, nil
}

// fakeScheduler implements scheduler.Scheduler with zero values. Name
// reports "embedded": TestScheduler_resolve pins the default adapter name.
type fakeScheduler struct{}

func (f *fakeScheduler) Schedule(_ context.Context, _, _ string, _ any) (scheduler.EntryID, error) {
	return 0, nil
}
func (f *fakeScheduler) Remove(_ scheduler.EntryID) error { return nil }
func (f *fakeScheduler) Entries() []scheduler.EntryID     { return nil }
func (f *fakeScheduler) Start() error                     { return nil }
func (f *fakeScheduler) Stop() error                      { return nil }
func (f *fakeScheduler) Name() string                     { return "embedded" }

func newFakeSearch(_ search.Options) (search.Search, error) { return &fakeSearch{}, nil }

// fakeSearch implements search.Search with zero values.
type fakeSearch struct{}

func (f *fakeSearch) Index(_ context.Context, _ search.Document) error { return nil }
func (f *fakeSearch) IndexBatch(_ context.Context, _ []search.Document) error {
	return nil
}
func (f *fakeSearch) Delete(_ context.Context, _ string) error { return nil }
func (f *fakeSearch) Search(_ context.Context, _ string, _ search.QueryOptions) (search.Result, error) {
	return search.Result{}, nil
}
func (f *fakeSearch) Close() error { return nil }

func newFakeVectorStore(_ vectorstore.Options) (vectorstore.VectorStore, error) {
	return &fakeVectorStore{}, nil
}

// fakeVectorStore implements vectorstore.VectorStore with zero values.
type fakeVectorStore struct{}

func (f *fakeVectorStore) Upsert(_ context.Context, _ vectorstore.Vector) error { return nil }
func (f *fakeVectorStore) UpsertBatch(_ context.Context, _ []vectorstore.Vector) error {
	return nil
}
func (f *fakeVectorStore) Delete(_ context.Context, _ string) error { return nil }
func (f *fakeVectorStore) Query(_ context.Context, _ []float32, _ int) ([]vectorstore.ScoreMatch, error) {
	return nil, nil
}
func (f *fakeVectorStore) Close() error { return nil }

func newFakeAnalytics(_ analytics.Options) (analytics.Analytics, error) {
	return &fakeAnalytics{}, nil
}

// fakeAnalytics implements analytics.Analytics with zero values.
type fakeAnalytics struct{}

func (f *fakeAnalytics) Track(_ context.Context, _ string, _ map[string]any) error {
	return nil
}
func (f *fakeAnalytics) Identify(_ context.Context, _ string, _ map[string]any) error {
	return nil
}
func (f *fakeAnalytics) Group(_ context.Context, _, _ string, _ map[string]any) error {
	return nil
}
func (f *fakeAnalytics) Close() error { return nil }

func newFakeCache(_ cache.Options) (cache.Cache, error) { return &fakeCache{}, nil }

// fakeCache implements cache.Cache with zero values.
type fakeCache struct{}

func (f *fakeCache) Get(_ context.Context, _ string) ([]byte, error) { return nil, nil }
func (f *fakeCache) Set(_ context.Context, _ string, _ []byte, _ time.Duration) error {
	return nil
}
func (f *fakeCache) SetIfAbsent(_ context.Context, _ string, _ []byte, _ time.Duration) (bool, error) {
	return false, nil
}
func (f *fakeCache) Delete(_ context.Context, _ string) error    { return nil }
func (f *fakeCache) Increment(_ context.Context, _ string) error { return nil }
func (f *fakeCache) Decrement(_ context.Context, _ string) error { return nil }
func (f *fakeCache) Exists(_ context.Context, _ string) (bool, error) {
	return false, nil
}
func (f *fakeCache) Close(_ context.Context) error { return nil }

func newFakeCrypto(_ crypto.Options) (crypto.Crypto, error) { return &fakeCrypto{}, nil }

// fakeCrypto implements crypto.Crypto with zero values.
type fakeCrypto struct{}

func (f *fakeCrypto) Encrypt(_ context.Context, _ []byte) ([]byte, error) { return nil, nil }
func (f *fakeCrypto) Decrypt(_ context.Context, _ []byte) ([]byte, error) { return nil, nil }
func (f *fakeCrypto) Sign(_ context.Context, _ []byte) ([]byte, error)    { return nil, nil }
func (f *fakeCrypto) Verify(_ context.Context, _, _ []byte) (bool, error) {
	return false, nil
}
func (f *fakeCrypto) Mac(_ context.Context, _ []byte) ([]byte, error) { return nil, nil }
func (f *fakeCrypto) VerifyMac(_ context.Context, _, _ []byte) (bool, error) {
	return false, nil
}

func newFakeEventBus(_ eventbus.Options) (eventbus.EventBus, error) {
	return &fakeEventBus{}, nil
}

// fakeEventBus implements eventbus.EventBus with zero values.
type fakeEventBus struct{}

func (f *fakeEventBus) Publish(_ context.Context, _ string, _ eventbus.Payload, _ eventbus.Headers) error {
	return nil
}
func (f *fakeEventBus) Subscribe(_ context.Context, _ string, _ eventbus.Handler) (func(), error) {
	return func() {}, nil
}
func (f *fakeEventBus) Close() error { return nil }
func (f *fakeEventBus) Name() string { return "fake" }
func (f *fakeEventBus) SubscribeChan(_ context.Context, _ string, _ int) (<-chan eventbus.Message, error) {
	ch := make(chan eventbus.Message)
	return ch, nil
}
func (f *fakeEventBus) Unsubscribe(_ string, _ <-chan eventbus.Message) error {
	return nil
}

func newFakeFlag(_ flag.Options) (flag.Flag, error) { return &fakeFlag{}, nil }

// fakeFlag implements flag.Flag with zero values.
type fakeFlag struct{}

func (f *fakeFlag) Bool(_ context.Context, _ string, _ bool) (bool, error) {
	return false, nil
}
func (f *fakeFlag) String(_ context.Context, _ string, _ string) (string, error) {
	return "", nil
}
func (f *fakeFlag) Int(_ context.Context, _ string, _ int) (int, error) {
	return 0, nil
}
func (f *fakeFlag) JSON(_ context.Context, _ string, _ any, _ any) error { return nil }
func (f *fakeFlag) Close() error                                         { return nil }

func newFakeGeo(_ geo.Options) (geo.Geo, error) { return &fakeGeo{}, nil }

// fakeGeo implements geo.Geo with zero values.
type fakeGeo struct{}

func (f *fakeGeo) Geocode(_ context.Context, _ string) ([]geo.Location, error) {
	return nil, nil
}
func (f *fakeGeo) ReverseGeocode(_ context.Context, _, _ float64) ([]geo.Address, error) {
	return nil, nil
}
func (f *fakeGeo) Distance(_ context.Context, _, _ geo.Point) (float64, error) {
	return 0, nil
}
func (f *fakeGeo) Close() error { return nil }

func newFakeI18n(_ i18n.Options) (i18n.I18n, error) { return &fakeI18n{}, nil }

// fakeI18n implements i18n.I18n with zero values.
type fakeI18n struct{}

func (f *fakeI18n) Translate(_ context.Context, _, _ string, _ map[string]string) (string, error) {
	return "", nil
}
func (f *fakeI18n) Locales(_ context.Context) ([]string, error) { return nil, nil }
func (f *fakeI18n) Close() error                                { return nil }

func newFakeIdempotency(_ idempotency.Options) (idempotency.Store, error) {
	return &fakeIdempotency{}, nil
}

// fakeIdempotency implements idempotency.Store with zero values.
type fakeIdempotency struct{}

func (f *fakeIdempotency) Begin(_ context.Context, _ string, _ idempotency.BeginOptions) (idempotency.Outcome, error) {
	return idempotency.Outcome{}, nil
}
func (f *fakeIdempotency) Complete(_ context.Context, _ string, _, _ []byte) error {
	return nil
}
func (f *fakeIdempotency) Forget(_ context.Context, _ string) error { return nil }
func (f *fakeIdempotency) Close() error                             { return nil }

func newFakeLock(_ lock.Options) (lock.Locker, error) { return &fakeLocker{}, nil }

// fakeLocker implements lock.Locker with zero values.
type fakeLocker struct{}

func (f *fakeLocker) TryAcquire(_ context.Context, _ string, _ time.Duration) (lock.Lock, bool, error) {
	return &fakeLock{}, false, nil
}
func (f *fakeLocker) Acquire(_ context.Context, _ string, _ time.Duration) (lock.Lock, error) {
	return &fakeLock{}, nil
}
func (f *fakeLocker) Close(_ context.Context) error { return nil }

// fakeLock implements lock.Lock with zero values.
type fakeLock struct{}

func (f *fakeLock) Key() string                                     { return "" }
func (f *fakeLock) Extend(_ context.Context, _ time.Duration) error { return nil }
func (f *fakeLock) Unlock(_ context.Context) error                  { return nil }

func newFakeLog(_ log.Options) (log.Logger, error) { return &fakeLogger{}, nil }

// fakeLogger implements log.Logger with zero values.
type fakeLogger struct{}

func (f *fakeLogger) Debug() log.Event                         { return &fakeLogEvent{} }
func (f *fakeLogger) Info() log.Event                          { return &fakeLogEvent{} }
func (f *fakeLogger) Warn() log.Event                          { return &fakeLogEvent{} }
func (f *fakeLogger) Error() log.Event                         { return &fakeLogEvent{} }
func (f *fakeLogger) Fatal() log.Event                         { return &fakeLogEvent{} }
func (f *fakeLogger) With() log.Context                        { return &fakeLogContext{} }
func (f *fakeLogger) WithContext(_ context.Context) log.Logger { return f }
func (f *fakeLogger) Enabled(_ log.Level) bool                 { return false }
func (f *fakeLogger) Sync() error                              { return nil }
func (f *fakeLogger) Name() string                             { return "fake" }

// fakeLogEvent implements log.Event with zero values.
type fakeLogEvent struct{}

func (e *fakeLogEvent) Str(_, _ string) log.Event               { return e }
func (e *fakeLogEvent) Int(_ string, _ int) log.Event           { return e }
func (e *fakeLogEvent) Int64(_ string, _ int64) log.Event       { return e }
func (e *fakeLogEvent) Float64(_ string, _ float64) log.Event   { return e }
func (e *fakeLogEvent) Bool(_ string, _ bool) log.Event         { return e }
func (e *fakeLogEvent) Dur(_ string, _ time.Duration) log.Event { return e }
func (e *fakeLogEvent) Time(_ string, _ time.Time) log.Event    { return e }
func (e *fakeLogEvent) Err(_ error) log.Event                   { return e }
func (e *fakeLogEvent) AnErr(_ string, _ error) log.Event       { return e }
func (e *fakeLogEvent) Any(_ string, _ any) log.Event           { return e }
func (e *fakeLogEvent) Msg(_ string)                            {}
func (e *fakeLogEvent) Msgf(_ string, _ ...any)                 {}
func (e *fakeLogEvent) Send()                                   {}

// fakeLogContext implements log.Context with zero values.
type fakeLogContext struct{}

func (c *fakeLogContext) Str(_, _ string) log.Context             { return c }
func (c *fakeLogContext) Int(_ string, _ int) log.Context         { return c }
func (c *fakeLogContext) Int64(_ string, _ int64) log.Context     { return c }
func (c *fakeLogContext) Float64(_ string, _ float64) log.Context { return c }
func (c *fakeLogContext) Bool(_ string, _ bool) log.Context       { return c }
func (c *fakeLogContext) Dur(_ string, _ time.Duration) log.Context {
	return c
}
func (c *fakeLogContext) Time(_ string, _ time.Time) log.Context { return c }
func (c *fakeLogContext) Err(_ error) log.Context                { return c }
func (c *fakeLogContext) AnErr(_ string, _ error) log.Context    { return c }
func (c *fakeLogContext) Any(_ string, _ any) log.Context        { return c }
func (c *fakeLogContext) Logger() log.Logger                     { return &fakeLogger{} }

func newFakeMailer(_ mailer.Options) (mailer.Mailer, error) { return &fakeMailer{}, nil }

// fakeMailer implements mailer.Mailer with zero values.
type fakeMailer struct{}

func (f *fakeMailer) Send(_ context.Context, _ *mailer.Mail) error { return nil }
func (f *fakeMailer) Close() error                                 { return nil }

func newFakeNotification(_ notification.Options) (notification.Notifier, error) {
	return &fakeNotification{}, nil
}

// fakeNotification implements notification.Notifier with zero values.
type fakeNotification struct{}

func (f *fakeNotification) Notify(_ context.Context, _ *notification.Notification) error {
	return nil
}
func (f *fakeNotification) Close() error { return nil }

func newFakeObservability(_ observability.Options) (observability.Provider, error) {
	return &fakeObservability{}, nil
}

// fakeObservability implements observability.Provider with zero values.
type fakeObservability struct{}

func (f *fakeObservability) Tracer(_ string) observability.Tracer { return &fakeTracer{} }
func (f *fakeObservability) Meter(_ string) observability.Metrics { return &fakeMetrics{} }
func (f *fakeObservability) Shutdown(_ context.Context) error     { return nil }

// fakeTracer implements observability.Tracer with zero values.
type fakeTracer struct{}

func (t *fakeTracer) Start(ctx context.Context, _ string) (context.Context, observability.Span) {
	return ctx, &fakeSpan{}
}
func (t *fakeTracer) Shutdown(_ context.Context) error { return nil }

// fakeMetrics implements observability.Metrics with zero values.
type fakeMetrics struct{}

func (m *fakeMetrics) Counter(_ context.Context, _ string, _ float64, _ ...observability.Attr) error {
	return nil
}
func (m *fakeMetrics) Gauge(_ context.Context, _ string, _ float64, _ ...observability.Attr) error {
	return nil
}
func (m *fakeMetrics) Histogram(_ context.Context, _ string, _ float64, _ ...observability.Attr) error {
	return nil
}
func (m *fakeMetrics) Shutdown(_ context.Context) error { return nil }

// fakeSpan implements observability.Span with zero values.
type fakeSpan struct{}

func (s *fakeSpan) SetAttributes(_ ...observability.Attr) {}
func (s *fakeSpan) RecordError(_ error)                   {}
func (s *fakeSpan) End()                                  {}

func newFakePayment(_ payment.Options) (payment.Payment, error) {
	return &fakePayment{}, nil
}

// fakePayment implements payment.Payment with zero values.
type fakePayment struct{}

func (f *fakePayment) CreatePayment(_ context.Context, _ payment.Request) (payment.Result, error) {
	return payment.Result{}, nil
}
func (f *fakePayment) Refund(_ context.Context, _ string, _ int64, _ string) error {
	return nil
}
func (f *fakePayment) GetPayment(_ context.Context, _ string) (payment.Result, error) {
	return payment.Result{}, nil
}
func (f *fakePayment) WebhookEvent(_ context.Context, _ []byte, _ string) (payment.Event, error) {
	return payment.Event{}, nil
}
func (f *fakePayment) Close() error { return nil }

func newFakePermission(_ permission.Options) (permission.Checker, error) {
	return &fakePermission{}, nil
}

// fakePermission implements permission.Checker with zero values.
type fakePermission struct{}

func (f *fakePermission) Can(_ context.Context, _ permission.Subject, _ string, _ permission.Resource) (permission.Decision, error) {
	return permission.Decision{}, nil
}

func newFakeQueue(_ queue.Options) (queue.Queue, error) { return &fakeQueue{}, nil }

// fakeQueue implements queue.Queue with zero values.
type fakeQueue struct{}

func (f *fakeQueue) Push(_ context.Context, _ string, _ queue.Payload, _ queue.Headers) error {
	return nil
}
func (f *fakeQueue) PushDelayed(_ context.Context, _ string, _ queue.Payload, _ queue.Headers, _ time.Duration) error {
	return nil
}
func (f *fakeQueue) Pop(_ context.Context, _ string) (queue.Message, error) {
	return queue.Message{}, nil
}
func (f *fakeQueue) Ack(_ context.Context, _ queue.Message) error { return nil }
func (f *fakeQueue) Nack(_ context.Context, _ queue.Message, _ bool) error {
	return nil
}
func (f *fakeQueue) Length(_ context.Context, _ string) (int64, error) { return 0, nil }
func (f *fakeQueue) IsEmpty(_ context.Context, _ string) (bool, error) {
	return true, nil
}
func (f *fakeQueue) Close() error { return nil }
func (f *fakeQueue) Name() string { return "fake" }

func newFakeRateLimit(_ ratelimit.Options) (ratelimit.Limiter, error) {
	return &fakeRateLimit{}, nil
}

// fakeRateLimit implements ratelimit.Limiter with zero values.
type fakeRateLimit struct{}

func (f *fakeRateLimit) Allow(_ context.Context, _ string, _ float64) (ratelimit.Decision, error) {
	return ratelimit.Decision{}, nil
}
func (f *fakeRateLimit) Reset(_ context.Context, _ string) error { return nil }
func (f *fakeRateLimit) Close() error                            { return nil }
func (f *fakeRateLimit) Name() string                            { return "fake" }

func newFakeRouter(_ router.Options) (router.Router, error) { return &fakeRouter{}, nil }

// fakeRouter implements router.Router and router.Group with zero values.
type fakeRouter struct{}

func (f *fakeRouter) Handle(_ string, _ string, _ http.HandlerFunc) {}
func (f *fakeRouter) Group(_ string, _ ...func(http.Handler) http.Handler) router.Group {
	return f
}
func (f *fakeRouter) Use(_ ...func(http.Handler) http.Handler)         {}
func (f *fakeRouter) ServeHTTP(_ http.ResponseWriter, _ *http.Request) {}

func newFakeSecrets(_ secrets.Options) (secrets.Secrets, error) {
	return &fakeSecrets{}, nil
}

// fakeSecrets implements secrets.Secrets with zero values.
type fakeSecrets struct{}

func (f *fakeSecrets) Get(_ context.Context, _ string) ([]byte, error) { return nil, nil }
func (f *fakeSecrets) Set(_ context.Context, _ string, _ []byte) error { return nil }
func (f *fakeSecrets) Delete(_ context.Context, _ string) error        { return nil }
func (f *fakeSecrets) List(_ context.Context) ([]string, error)        { return nil, nil }
func (f *fakeSecrets) Close(_ context.Context) error                   { return nil }

func newFakeSession(_ session.Options) (session.Store, error) {
	return &fakeSession{}, nil
}

// fakeSession implements session.Store with zero values.
type fakeSession struct{}

func (f *fakeSession) Create(_ context.Context, _ time.Duration) (session.Session, error) {
	return session.Session{}, nil
}
func (f *fakeSession) Get(_ context.Context, _ string) (session.Session, error) {
	return session.Session{}, nil
}
func (f *fakeSession) Save(_ context.Context, _ session.Session) error { return nil }
func (f *fakeSession) Delete(_ context.Context, _ string) error        { return nil }
func (f *fakeSession) Close() error                                    { return nil }

func newFakeStorage(_ storage.Options) (storage.Storage, error) {
	return &fakeStorage{}, nil
}

// fakeStorage implements storage.Storage with zero values.
type fakeStorage struct{}

func (f *fakeStorage) PresignUpload(_ context.Context, _, _, _ string, _ time.Duration) (storage.PresignedURL, error) {
	return storage.PresignedURL{}, nil
}
func (f *fakeStorage) PresignDownload(_ context.Context, _, _ string, _ time.Duration) (storage.PresignedURL, error) {
	return storage.PresignedURL{}, nil
}
func (f *fakeStorage) Exists(_ context.Context, _, _ string) (bool, error) {
	return false, nil
}
func (f *fakeStorage) Delete(_ context.Context, _, _ string) error { return nil }
func (f *fakeStorage) Move(_ context.Context, _, _, _, _ string) error {
	return nil
}
func (f *fakeStorage) Close(_ context.Context) error { return nil }
func (f *fakeStorage) Name() string                  { return "fake" }

func newFakeTenant(_ tenant.Options) (tenant.Tenant, error) { return &fakeTenant{}, nil }

// fakeTenant implements tenant.Tenant with zero values.
type fakeTenant struct{}

func (f *fakeTenant) Resolve(_ context.Context, _ map[string]string) (string, error) {
	return "", nil
}
func (f *fakeTenant) Scoped(ctx context.Context, _ string) (context.Context, error) {
	return ctx, nil
}
func (f *fakeTenant) Close() error { return nil }

func newFakeWebhook(_ webhook.Options) (webhook.Webhook, error) {
	return &fakeWebhook{}, nil
}

// fakeWebhook implements webhook.Webhook with zero values.
type fakeWebhook struct{}

func (f *fakeWebhook) Register(_ context.Context, _, _, _ string) error    { return nil }
func (f *fakeWebhook) Unregister(_ context.Context, _, _ string) error     { return nil }
func (f *fakeWebhook) Deliver(_ context.Context, _ string, _ []byte) error { return nil }
func (f *fakeWebhook) Close() error                                        { return nil }

func newFakeWorkflow(_ workflow.Options) (workflow.Workflow, error) {
	return &fakeWorkflow{}, nil
}

// fakeWorkflow implements workflow.Workflow with zero values.
type fakeWorkflow struct{}

func (f *fakeWorkflow) Start(_ context.Context, _ string, _ any, _ string) (workflow.RunID, error) {
	return "", nil
}
func (f *fakeWorkflow) Signal(_ context.Context, _ workflow.RunID, _ string, _ any) error {
	return nil
}
func (f *fakeWorkflow) Query(_ context.Context, _ workflow.RunID, _ string, _ any) error {
	return nil
}
func (f *fakeWorkflow) Cancel(_ context.Context, _ workflow.RunID) error { return nil }
func (f *fakeWorkflow) Close() error                                     { return nil }

var (
	_ ai.AI                   = (*fakeAI)(nil)
	_ analytics.Analytics     = (*fakeAnalytics)(nil)
	_ auth.Auth               = (*fakeAuth)(nil)
	_ billing.Billing         = (*fakeBilling)(nil)
	_ cache.Cache             = (*fakeCache)(nil)
	_ crypto.Crypto           = (*fakeCrypto)(nil)
	_ db.DB                   = (*fakeDB)(nil)
	_ db.Transactor           = (*fakeDB)(nil)
	_ db.Tx                   = (*fakeTx)(nil)
	_ document.Document       = (*fakeDocument)(nil)
	_ eventbus.EventBus       = (*fakeEventBus)(nil)
	_ flag.Flag               = (*fakeFlag)(nil)
	_ geo.Geo                 = (*fakeGeo)(nil)
	_ i18n.I18n               = (*fakeI18n)(nil)
	_ idempotency.Store       = (*fakeIdempotency)(nil)
	_ lock.Locker             = (*fakeLocker)(nil)
	_ lock.Lock               = (*fakeLock)(nil)
	_ log.Logger              = (*fakeLogger)(nil)
	_ log.Event               = (*fakeLogEvent)(nil)
	_ log.Context             = (*fakeLogContext)(nil)
	_ mailer.Mailer           = (*fakeMailer)(nil)
	_ media.Media             = (*fakeMedia)(nil)
	_ notification.Notifier   = (*fakeNotification)(nil)
	_ observability.Provider  = (*fakeObservability)(nil)
	_ observability.Tracer    = (*fakeTracer)(nil)
	_ observability.Metrics   = (*fakeMetrics)(nil)
	_ observability.Span      = (*fakeSpan)(nil)
	_ password.Hasher         = (*fakePassword)(nil)
	_ payment.Payment         = (*fakePayment)(nil)
	_ permission.Checker      = (*fakePermission)(nil)
	_ queue.Queue             = (*fakeQueue)(nil)
	_ ratelimit.Limiter       = (*fakeRateLimit)(nil)
	_ router.Router           = (*fakeRouter)(nil)
	_ router.Group            = (*fakeRouter)(nil)
	_ scheduler.Scheduler     = (*fakeScheduler)(nil)
	_ search.Search           = (*fakeSearch)(nil)
	_ secrets.Secrets         = (*fakeSecrets)(nil)
	_ session.Store           = (*fakeSession)(nil)
	_ storage.Storage         = (*fakeStorage)(nil)
	_ tenant.Tenant           = (*fakeTenant)(nil)
	_ vectorstore.VectorStore = (*fakeVectorStore)(nil)
	_ webhook.Webhook         = (*fakeWebhook)(nil)
	_ workflow.Workflow       = (*fakeWorkflow)(nil)
)

// closeContainer shuts down resolved services so the package goleak check
// sees no leftover background work (cache janitors, queues, servers).
func closeContainer(t *testing.T, c *Container) {
	t.Helper()

	t.Cleanup(func() {
		if err := c.Close(t.Context()); err != nil {
			t.Logf("close: %v", err)
		}
	})
}

// sameInstance fails the test when two resolutions of one accessor are not
// the same singleton instance.
func sameInstance(t *testing.T, a, b any) {
	t.Helper()

	va, vb := reflect.ValueOf(a), reflect.ValueOf(b)
	if !va.IsValid() || !vb.IsValid() {
		t.Fatal("nil service instance")
	}

	if va.Type() != vb.Type() {
		t.Fatalf("type changed between calls: %T vs %T", a, b)
	}

	if va.Kind() == reflect.Pointer {
		if va.Pointer() != vb.Pointer() {
			t.Error("singleton identity violated: distinct pointers")
		}

		return
	}

	if a != b {
		t.Error("singleton identity violated: distinct values")
	}
}

func TestAccessors_resolve(t *testing.T) {
	t.Parallel()

	c := New(testConfig(t))
	closeContainer(t, c)

	tests := []struct {
		name    string
		resolve func() (any, error)
	}{
		{"ai", func() (any, error) { return c.AI() }},
		{"analytics", func() (any, error) { return c.Analytics() }},
		{"auth", func() (any, error) { return c.Auth() }},
		{"billing", func() (any, error) { return c.Billing() }},
		{"cache", func() (any, error) { return c.Cache() }},
		{"crypto", func() (any, error) { return c.Crypto() }},
		{"db", func() (any, error) { return c.DB() }},
		{"document", func() (any, error) { return c.Document() }},
		{"eventbus", func() (any, error) { return c.EventBus() }},
		{"flag", func() (any, error) { return c.Flag() }},
		{"geo", func() (any, error) { return c.Geo() }},
		{"i18n", func() (any, error) { return c.I18n() }},
		{"idempotency", func() (any, error) { return c.Idempotency() }},
		{"lock", func() (any, error) { return c.Lock() }},
		{"log", func() (any, error) { return c.Log() }},
		{"mailer", func() (any, error) { return c.Mailer() }},
		{"media", func() (any, error) { return c.Media() }},
		{"notification", func() (any, error) { return c.Notification() }},
		{"observability", func() (any, error) { return c.Observability() }},
		{"password", func() (any, error) { return c.Password() }},
		{"payment", func() (any, error) { return c.Payment() }},
		{"permission", func() (any, error) { return c.Permission() }},
		{"queue", func() (any, error) { return c.Queue() }},
		{"ratelimit", func() (any, error) { return c.RateLimit() }},
		{"router", func() (any, error) { return c.Router() }},
		{"search", func() (any, error) { return c.Search() }},
		{"secrets", func() (any, error) { return c.Secrets() }},
		{"session", func() (any, error) { return c.Session() }},
		{"storage", func() (any, error) { return c.Storage() }},
		{"tenant", func() (any, error) { return c.Tenant() }},
		{"vectorstore", func() (any, error) { return c.VectorStore() }},
		{"webhook", func() (any, error) { return c.Webhook() }},
		{"workflow", func() (any, error) { return c.Workflow() }},
	}

	// Sequential: subtests share one container, and the parent's cleanup
	// Close must run after all of them finish.
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			v, err := tt.resolve()
			if err != nil {
				t.Fatalf("resolve failed: %v", err)
			}

			if v == nil {
				t.Fatal("resolve returned nil instance")
			}

			sameInstance(t, v, mustResolve(t, tt.resolve))
		})
	}
}

func TestAccessors_alternateAdapters(t *testing.T) {
	t.Parallel()

	// Covers the infallible factory wrappers (log noop/slog/pretty,
	// observability noop) that defaults never exercise.
	tests := []struct {
		name    string
		mutate  func(cfg *config.Config)
		resolve func(c *Container) (any, error)
	}{
		{"log/noop", func(cfg *config.Config) { cfg.Log.Adapter = "noop" }, func(c *Container) (any, error) { return c.Log() }},
		{"log/slog", func(cfg *config.Config) { cfg.Log.Adapter = "slog" }, func(c *Container) (any, error) { return c.Log() }},
		{"log/pretty", func(cfg *config.Config) { cfg.Log.Adapter = "pretty" }, func(c *Container) (any, error) { return c.Log() }},
		{"observability/noop", func(cfg *config.Config) { cfg.Observability.Adapter = "noop" }, func(c *Container) (any, error) { return c.Observability() }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := testConfig(t)
			tt.mutate(cfg)

			c := New(cfg)
			closeContainer(t, c)

			v, err := tt.resolve(c)
			if err != nil {
				t.Fatalf("resolve failed: %v", err)
			}

			if v == nil {
				t.Fatal("resolve returned nil instance")
			}
		})
	}
}

func mustResolve(t *testing.T, resolve func() (any, error)) any {
	t.Helper()

	v, err := resolve()
	if err != nil {
		t.Fatalf("second resolve failed: %v", err)
	}

	return v
}

func TestAccessors_lazy(t *testing.T) {
	t.Parallel()

	c := New(testConfig(t))
	closeContainer(t, c)

	if c.ai.resolved() || c.db.resolved() || c.cache.resolved() {
		t.Fatal("fresh container has resolved services")
	}

	if _, err := c.AI(); err != nil {
		t.Fatalf("AI() failed: %v", err)
	}

	if !c.ai.resolved() {
		t.Error("AI() did not mark ai resolved")
	}

	if c.db.resolved() || c.cache.resolved() {
		t.Error("resolving AI resolved unrelated services")
	}
}

func TestAccessors_badAdapterThenRetry(t *testing.T) {
	t.Parallel()

	c := New(testConfig(t))
	closeContainer(t, c)

	c.cfg.DB.Adapter = "bogus-adapter"

	if _, err := c.DB(); err == nil {
		t.Fatal("expected error for bogus db adapter, got nil")
	}

	if c.db.resolved() {
		t.Fatal("failed build must not mark the service resolved")
	}

	c.cfg.DB.Adapter = "sqlite"

	if _, err := c.DB(); err != nil {
		t.Fatalf("retry after config fix failed: %v", err)
	}
}

func TestAccessors_badAdapterError(t *testing.T) {
	t.Parallel()

	c := New(testConfig(t))
	closeContainer(t, c)

	c.cfg.Cache.Adapter = "bogus-adapter"

	_, err := c.Cache()
	if err == nil {
		t.Fatal("expected error for bogus cache adapter, got nil")
	}

	if !strings.Contains(err.Error(), "cache") {
		t.Errorf("error should name the service, got: %v", err)
	}
}

func TestCrypto_failureCarriesNoSecretText(t *testing.T) {
	t.Parallel()

	const marker = "s3cr3t-marker-zz9"

	c := New(testConfig(t))
	closeContainer(t, c)

	c.cfg.Crypto.Options.Key = "NOT-BASE64!!-" + marker

	_, err := c.Crypto()
	if err == nil {
		t.Fatal("expected error for invalid crypto key, got nil")
	}

	if strings.Contains(err.Error(), marker) {
		t.Errorf("error leaks secret text: %v", err)
	}
}

func TestAccessors_concurrent(t *testing.T) {
	t.Parallel()

	c := New(testConfig(t))
	closeContainer(t, c)

	const n = 8

	results := make([]any, n)
	errs := make([]error, n)

	var wg sync.WaitGroup

	for i := range n {
		wg.Add(1)

		go func() {
			defer wg.Done()

			v, err := c.Cache()
			results[i] = v
			errs[i] = err
		}()
	}

	wg.Wait()

	for i := range n {
		if errs[i] != nil {
			t.Fatalf("goroutine %d failed: %v", i, errs[i])
		}

		sameInstance(t, results[0], results[i])
	}
}

func TestJob_sharesQueue(t *testing.T) {
	t.Parallel()

	c := New(testConfig(t))
	closeContainer(t, c)

	q, err := c.Queue()
	if err != nil {
		t.Fatalf("Queue() failed: %v", err)
	}

	d, err := c.Job()
	if err != nil {
		t.Fatalf("Job() failed: %v", err)
	}

	if d == nil {
		t.Fatal("Job() returned nil dispatcher")
	}

	if d.Q != q {
		t.Error("dispatcher does not share the resolved queue instance")
	}

	again, err := c.Job()
	if err != nil {
		t.Fatalf("second Job() failed: %v", err)
	}

	if again != d {
		t.Error("Job() is not a singleton")
	}
}

func TestJob_queueFailure(t *testing.T) {
	t.Parallel()

	c := New(testConfig(t))
	closeContainer(t, c)

	c.cfg.Queue.Adapter = "bogus-adapter"

	if _, err := c.Job(); err == nil {
		t.Fatal("expected Job error for bogus queue adapter, got nil")
	}
}

func TestScheduler_resolve(t *testing.T) {
	t.Parallel()

	c := New(testConfig(t))
	closeContainer(t, c)

	s, err := c.Scheduler()
	if err != nil {
		t.Fatalf("Scheduler() failed: %v", err)
	}

	if s == nil {
		t.Fatal("Scheduler() returned nil")
	}

	if got := s.Name(); got != "embedded" {
		t.Errorf("Name() = %q, want %q", got, "embedded")
	}

	again, err := c.Scheduler()
	if err != nil {
		t.Fatalf("second Scheduler() failed: %v", err)
	}

	if again != s {
		t.Error("Scheduler() is not a singleton")
	}
}

func TestScheduler_injectsJobDispatcher(t *testing.T) {
	t.Parallel()

	cfg := testConfig(t)
	cfg.Scheduler.Options.Dispatcher = nil

	c := New(cfg)
	closeContainer(t, c)

	s, err := c.Scheduler()
	if err != nil {
		t.Fatalf("Scheduler() with nil dispatcher failed: %v", err)
	}

	if s == nil {
		t.Fatal("Scheduler() returned nil")
	}

	if !c.job.resolved() {
		t.Error("nil dispatcher should resolve the shared job dispatcher")
	}
}

func TestScheduler_jobFailure(t *testing.T) {
	t.Parallel()

	cfg := testConfig(t)
	cfg.Scheduler.Options.Dispatcher = nil
	cfg.Queue.Adapter = "bogus-adapter"

	c := New(cfg)
	closeContainer(t, c)

	if _, err := c.Scheduler(); err == nil {
		t.Fatal("expected Scheduler error for bogus queue adapter, got nil")
	}
}

func TestScheduler_badAdapter(t *testing.T) {
	t.Parallel()

	c := New(testConfig(t))
	closeContainer(t, c)

	c.cfg.Scheduler.Adapter = "bogus-adapter"

	if _, err := c.Scheduler(); err == nil {
		t.Fatal("expected error for bogus scheduler adapter, got nil")
	}
}

func TestTransactor_ok(t *testing.T) {
	t.Parallel()

	c := New(testConfig(t))
	closeContainer(t, c)

	tr, err := c.Transactor()
	if err != nil {
		t.Fatalf("Transactor() failed: %v", err)
	}

	if tr == nil {
		t.Fatal("Transactor() returned nil")
	}
}

func TestTransactor_dbFailure(t *testing.T) {
	t.Parallel()

	c := New(testConfig(t))
	closeContainer(t, c)

	c.cfg.DB.Adapter = "bogus-adapter"

	if _, err := c.Transactor(); err == nil {
		t.Fatal("expected Transactor error for bogus db adapter, got nil")
	}
}

// stubDB implements db.DB by embedding it but never Transactor, so
// Transactor must reject it. Seeded through lazy.get, which is the only
// same-package seam the frozen lazy contract exposes.
type stubDB struct {
	db.DB
}

func TestTransactor_unsupported(t *testing.T) {
	t.Parallel()

	c := New(testConfig(t))
	closeContainer(t, c)

	if _, err := c.db.get(func() (db.DB, error) { return stubDB{}, nil }); err != nil {
		t.Fatalf("seeding stub db failed: %v", err)
	}

	_, err := c.Transactor()
	if err == nil {
		t.Fatal("expected Transactor error for non-transactor db, got nil")
	}

	var terr TransactorError
	if !errors.As(err, &terr) {
		t.Fatalf("error is %T, want TransactorError", err)
	}

	if !strings.Contains(terr.Actual, "stubDB") {
		t.Errorf("TransactorError.Actual = %q, want it to name stubDB", terr.Actual)
	}

	if !errors.Is(err, ErrTransactorUnsupported) {
		t.Errorf("error should unwrap to ErrTransactorUnsupported, got: %v", err)
	}
}

func TestGRPC_singletonOptsIgnored(t *testing.T) {
	t.Parallel()

	c := New(testConfig(t))
	closeContainer(t, c)

	if c.grpcServer.resolved() {
		t.Fatal("fresh container has grpc server resolved")
	}

	first, err := c.GRPC()
	if err != nil {
		t.Fatalf("GRPC() failed: %v", err)
	}

	if first == nil {
		t.Fatal("GRPC() returned nil server")
	}

	second, err := c.GRPC(grpc.MaxRecvMsgSize(1))
	if err != nil {
		t.Fatalf("second GRPC() failed: %v", err)
	}

	if second != first {
		t.Error("GRPC() is not a singleton: later options built a new server")
	}
}
