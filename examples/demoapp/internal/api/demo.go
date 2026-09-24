package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/zenta-dev/zever/ai"
	"github.com/zenta-dev/zever/cache"
	"github.com/zenta-dev/zever/document"
	"github.com/zenta-dev/zever/eventbus"
	"github.com/zenta-dev/zever/idempotency"
	"github.com/zenta-dev/zever/media"
	"github.com/zenta-dev/zever/notification"
	"github.com/zenta-dev/zever/permission"
	"github.com/zenta-dev/zever/queue"
	"github.com/zenta-dev/zever/router"
	"github.com/zenta-dev/zever/search"
	"github.com/zenta-dev/zever/session"
	"github.com/zenta-dev/zever/vectorstore"
	"github.com/zenta-dev/zever/workflow"
)

// cacheGet returns the cached value for key, 404 on miss.
func (s *Server) cacheGet(w http.ResponseWriter, req *http.Request) {
	if s.Cache == nil {
		writeError(w, http.StatusNotImplemented, "cache not configured")
		return
	}
	key := router.Param(req, "key")
	val, err := s.Cache.Get(req.Context(), key)
	if err != nil {
		if errors.Is(err, cache.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "cache error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"key": key, "value": string(val)})
}

// cacheSet stores the raw request body under key.
func (s *Server) cacheSet(w http.ResponseWriter, req *http.Request) {
	if s.Cache == nil {
		writeError(w, http.StatusNotImplemented, "cache not configured")
		return
	}
	key := router.Param(req, "key")
	body, _ := io.ReadAll(req.Body)
	var ttl time.Duration
	if raw := req.URL.Query().Get("ttl"); raw != "" {
		ttl, _ = time.ParseDuration(raw)
	}
	if err := s.Cache.Set(req.Context(), key, body, ttl); err != nil {
		writeError(w, http.StatusInternalServerError, "cache error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// flagGet evaluates a flag as bool, falling back to string.
func (s *Server) flagGet(w http.ResponseWriter, req *http.Request) {
	if s.Flag == nil {
		writeError(w, http.StatusNotImplemented, "flag not configured")
		return
	}
	key := router.Param(req, "key")
	if b, err := s.Flag.Bool(req.Context(), key, false); err == nil {
		writeJSON(w, http.StatusOK, map[string]any{"key": key, "value": b, "type": "bool"})
		return
	}
	if v, err := s.Flag.String(req.Context(), key, ""); err == nil {
		writeJSON(w, http.StatusOK, map[string]any{"key": key, "value": v, "type": "string"})
		return
	}
	writeError(w, http.StatusNotFound, "flag not found")
}

type permissionCheckRequest struct {
	Subject  string `json:"subject"`
	Role     string `json:"role"`
	Resource string `json:"resource"`
	Action   string `json:"action"`
}

// permissionCheck evaluates the RBAC rules for one subject/action/resource.
func (s *Server) permissionCheck(w http.ResponseWriter, req *http.Request) {
	var body permissionCheckRequest
	if err := decodeJSON(req, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if s.Permission == nil {
		writeError(w, http.StatusNotImplemented, "permission not configured")
		return
	}
	decision, err := s.Permission.Can(req.Context(),
		permission.Subject{ID: body.Subject, Roles: []string{body.Role}},
		body.Action, permission.Resource{Type: body.Resource, ID: "*"})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "permission error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"allowed": decision.Allowed})
}

// ratelimitCheck consumes one token for key.
func (s *Server) ratelimitCheck(w http.ResponseWriter, req *http.Request) {
	if s.RateLimit == nil {
		writeError(w, http.StatusNotImplemented, "ratelimit not configured")
		return
	}
	decision, err := s.RateLimit.Allow(req.Context(), router.Param(req, "key"), 1)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ratelimit error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"allowed": decision.Allowed, "retry_after": decision.RetryAfter.String()})
}

// lockDemo acquires a 10s lock for key and releases it.
func (s *Server) lockDemo(w http.ResponseWriter, req *http.Request) {
	if s.Lock == nil {
		writeError(w, http.StatusNotImplemented, "lock not configured")
		return
	}
	key := router.Param(req, "key")
	ctx := req.Context()
	lk, ok, err := s.Lock.TryAcquire(ctx, key, 10*time.Second)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lock error")
		return
	}
	if !ok {
		writeJSON(w, http.StatusConflict, map[string]any{"acquired": false})
		return
	}
	defer func() { _ = lk.Unlock(ctx) }()
	writeJSON(w, http.StatusOK, map[string]any{"acquired": true, "key": key})
}

type idempotencyRequest struct {
	Data string `json:"data"`
}

// idempotencyDemo reserves key scoped to the request fingerprint. The first
// caller executes (201), a replay with the same fingerprint returns the
// stored result without re-executing.
func (s *Server) idempotencyDemo(w http.ResponseWriter, req *http.Request) {
	if s.Idempotency == nil {
		writeError(w, http.StatusNotImplemented, "idempotency not configured")
		return
	}
	var body idempotencyRequest
	if err := decodeJSON(req, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	key := router.Param(req, "key")
	outcome, err := s.Idempotency.Begin(req.Context(), key, idempotency.BeginOptions{
		Fingerprint: []byte(body.Data),
		TTL:         5 * time.Minute,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "idempotency error")
		return
	}
	if outcome.Replay {
		writeJSON(w, http.StatusOK, map[string]any{"replay": true, "result": string(outcome.Result)})
		return
	}
	result := []byte(`{"stored":` + strconv.Quote(body.Data) + `}`)
	if err := s.Idempotency.Complete(req.Context(), key, []byte(body.Data), result); err != nil {
		writeError(w, http.StatusInternalServerError, "idempotency error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"replay": false})
}

// sessionCreate mints a session carrying the posted data.
func (s *Server) sessionCreate(w http.ResponseWriter, req *http.Request) {
	if s.Session == nil {
		writeError(w, http.StatusNotImplemented, "session not configured")
		return
	}
	var data map[string]any
	if err := decodeJSON(req, &data); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	sess, err := s.Session.Create(req.Context(), time.Hour)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "session error")
		return
	}
	sess.Data = data
	if err := s.Session.Save(req.Context(), sess); err != nil {
		writeError(w, http.StatusInternalServerError, "session error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": sess.ID})
}

// sessionGet returns the session data for id. Unknown or expired ids read
// as 404; malformed ids read as 400.
func (s *Server) sessionGet(w http.ResponseWriter, req *http.Request) {
	if s.Session == nil {
		writeError(w, http.StatusNotImplemented, "session not configured")
		return
	}
	sess, err := s.Session.Get(req.Context(), router.Param(req, "id"))
	if err != nil {
		if errors.Is(err, session.ErrInvalidID) {
			writeError(w, http.StatusBadRequest, "malformed session id")
			return
		}
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, sess.Data)
}

// queuePush enqueues the raw request body on topic.
func (s *Server) queuePush(w http.ResponseWriter, req *http.Request) {
	if s.Queue == nil {
		writeError(w, http.StatusNotImplemented, "queue not configured")
		return
	}
	topic := router.Param(req, "topic")
	body, _ := io.ReadAll(req.Body)
	if err := s.Queue.Push(req.Context(), topic, queue.Payload(body), nil); err != nil {
		writeError(w, http.StatusInternalServerError, "queue error")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"topic": topic, "pushed": true})
}

type eventbusPublishRequest struct {
	Topic   string `json:"topic"`
	Payload string `json:"payload"`
}

// eventbusPublish fans payload out to topic subscribers.
func (s *Server) eventbusPublish(w http.ResponseWriter, req *http.Request) {
	var body eventbusPublishRequest
	if err := decodeJSON(req, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if s.EventBus == nil {
		writeError(w, http.StatusNotImplemented, "eventbus not configured")
		return
	}
	if err := s.EventBus.Publish(req.Context(), body.Topic, eventbus.Payload(body.Payload), nil); err != nil {
		writeError(w, http.StatusInternalServerError, "eventbus error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"published": true})
}

type searchIndexRequest struct {
	ID      string `json:"id"`
	Content string `json:"content"`
}

// searchIndex adds a document to the demo index.
func (s *Server) searchIndex(w http.ResponseWriter, req *http.Request) {
	var body searchIndexRequest
	if err := decodeJSON(req, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if s.Search == nil {
		writeError(w, http.StatusNotImplemented, "search not configured")
		return
	}
	if body.ID == "" {
		body.ID = uuid.NewString()
	}
	if err := s.Search.Index(req.Context(), search.Document{ID: body.ID, Index: "demo", Content: body.Content}); err != nil {
		writeError(w, http.StatusInternalServerError, "search error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"indexed": body.ID})
}

// searchQuery searches the demo index.
func (s *Server) searchQuery(w http.ResponseWriter, req *http.Request) {
	if s.Search == nil {
		writeError(w, http.StatusNotImplemented, "search not configured")
		return
	}
	res, err := s.Search.Search(req.Context(), req.URL.Query().Get("q"),
		search.QueryOptions{Limit: 10, Filters: map[string]string{"index": "demo"}})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search error")
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type vectorUpsertRequest struct {
	ID       string         `json:"id"`
	Vector   []float32      `json:"vector"`
	Metadata map[string]any `json:"metadata"`
}

// vectorUpsert stores an 8-dimensional demo embedding.
func (s *Server) vectorUpsert(w http.ResponseWriter, req *http.Request) {
	var body vectorUpsertRequest
	if err := decodeJSON(req, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if s.VectorStore == nil {
		writeError(w, http.StatusNotImplemented, "vectorstore not configured")
		return
	}
	if body.ID == "" {
		body.ID = uuid.NewString()
	}
	if len(body.Vector) == 0 {
		body.Vector = []float32{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8}
	}
	if err := s.VectorStore.Upsert(req.Context(), vectorstore.Vector{ID: body.ID, Embedding: body.Vector, Metadata: body.Metadata}); err != nil {
		writeError(w, http.StatusInternalServerError, "vectorstore error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"upserted": body.ID})
}

type vectorQueryRequest struct {
	Vector []float32 `json:"vector"`
	TopK   int       `json:"top_k"`
}

// vectorQuery returns the closest stored embeddings.
func (s *Server) vectorQuery(w http.ResponseWriter, req *http.Request) {
	var body vectorQueryRequest
	if err := decodeJSON(req, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if s.VectorStore == nil {
		writeError(w, http.StatusNotImplemented, "vectorstore not configured")
		return
	}
	if body.TopK <= 0 {
		body.TopK = 5
	}
	if len(body.Vector) == 0 {
		body.Vector = []float32{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8}
	}
	matches, err := s.VectorStore.Query(req.Context(), body.Vector, body.TopK)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "vectorstore error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"matches": matches})
}

type storagePresignRequest struct {
	Bucket string `json:"bucket"`
	Key    string `json:"key"`
}

// storagePresign mints an upload URL for bucket/key.
func (s *Server) storagePresign(w http.ResponseWriter, req *http.Request) {
	var body storagePresignRequest
	if err := decodeJSON(req, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if s.Storage == nil {
		writeError(w, http.StatusNotImplemented, "storage not configured")
		return
	}
	if body.Bucket == "" {
		body.Bucket = "demo"
	}
	if body.Key == "" {
		body.Key = uuid.NewString()
	}
	url, err := s.Storage.PresignUpload(req.Context(), body.Bucket, body.Key, "application/octet-stream", time.Hour)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "storage error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"url": url.URL, "bucket": body.Bucket, "key": body.Key})
}

// mediaUpload stores the raw request body as a media asset.
func (s *Server) mediaUpload(w http.ResponseWriter, req *http.Request) {
	if s.Media == nil {
		writeError(w, http.StatusNotImplemented, "media not configured")
		return
	}
	body, _ := io.ReadAll(req.Body)
	if len(body) == 0 {
		body = []byte("demo image data")
	}
	asset, err := s.Media.Upload(req.Context(), "demo/"+uuid.NewString()+".bin", body,
		media.UploadOptions{ContentType: "application/octet-stream"})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "media error")
		return
	}
	writeJSON(w, http.StatusOK, asset)
}

type aiGenerateRequest struct {
	Prompt string `json:"prompt"`
	Model  string `json:"model"`
}

// aiGenerate runs one LLM turn. Without provider credentials it answers 502;
// the server stays up either way.
func (s *Server) aiGenerate(w http.ResponseWriter, req *http.Request) {
	var body aiGenerateRequest
	if err := decodeJSON(req, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if s.AI == nil {
		writeError(w, http.StatusNotImplemented, "ai not configured")
		return
	}
	if body.Prompt == "" {
		body.Prompt = "Hello from demoapp"
	}
	if body.Model == "" {
		body.Model = "claude-haiku-4-5"
	}
	gen, err := s.AI.Generate(req.Context(), body.Model,
		[]ai.Message{{Role: ai.RoleUser, Content: body.Prompt}}, ai.GenerateOptions{})
	if err != nil {
		writeError(w, http.StatusBadGateway, "ai error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"content": gen.Content})
}

type geoGeocodeRequest struct {
	Address string `json:"address"`
}

// geoGeocode resolves an address against the static cities fixture.
func (s *Server) geoGeocode(w http.ResponseWriter, req *http.Request) {
	var body geoGeocodeRequest
	if err := decodeJSON(req, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if s.Geo == nil {
		writeError(w, http.StatusNotImplemented, "geo not configured")
		return
	}
	if body.Address == "" {
		body.Address = "Springfield"
	}
	locs, err := s.Geo.Geocode(req.Context(), body.Address)
	if err != nil {
		writeError(w, http.StatusNotFound, "no match")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": locs})
}

// i18nTranslate renders key for locale, interpolating Name=Demo.
func (s *Server) i18nTranslate(w http.ResponseWriter, req *http.Request) {
	if s.I18n == nil {
		writeError(w, http.StatusNotImplemented, "i18n not configured")
		return
	}
	locale := router.Param(req, "locale")
	key := router.Param(req, "key")
	msg, err := s.I18n.Translate(req.Context(), locale, key, map[string]string{"Name": "Demo"})
	if err != nil {
		writeError(w, http.StatusNotFound, "no message")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"locale": locale, "key": key, "message": msg})
}

type cryptoEncryptRequest struct {
	Plaintext string `json:"plaintext"`
}

// cryptoEncrypt seals plaintext with the local key.
func (s *Server) cryptoEncrypt(w http.ResponseWriter, req *http.Request) {
	var body cryptoEncryptRequest
	if err := decodeJSON(req, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if s.Crypto == nil {
		writeError(w, http.StatusNotImplemented, "crypto not configured")
		return
	}
	ct, err := s.Crypto.Encrypt(req.Context(), []byte(body.Plaintext))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "crypto error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ciphertext": ct})
}

type cryptoDecryptRequest struct {
	Ciphertext []byte `json:"ciphertext"`
}

// cryptoDecrypt opens ciphertext sealed by cryptoEncrypt.
func (s *Server) cryptoDecrypt(w http.ResponseWriter, req *http.Request) {
	var body cryptoDecryptRequest
	if err := decodeJSON(req, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if s.Crypto == nil {
		writeError(w, http.StatusNotImplemented, "crypto not configured")
		return
	}
	pt, err := s.Crypto.Decrypt(req.Context(), body.Ciphertext)
	if err != nil {
		writeError(w, http.StatusBadRequest, "decrypt failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"plaintext": string(pt)})
}

// secretsGet reads a name from the env-backed secrets store.
func (s *Server) secretsGet(w http.ResponseWriter, req *http.Request) {
	if s.Secrets == nil {
		writeError(w, http.StatusNotImplemented, "secrets not configured")
		return
	}
	name := router.Param(req, "name")
	val, err := s.Secrets.Get(req.Context(), name)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": name, "value": string(val)})
}

type notificationRequest struct {
	Target string `json:"target"`
	Title  string `json:"title"`
	Body   string `json:"body"`
}

// notificationSend delivers a push notification via the log adapter.
func (s *Server) notificationSend(w http.ResponseWriter, req *http.Request) {
	var body notificationRequest
	if err := decodeJSON(req, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if s.Notification == nil {
		writeError(w, http.StatusNotImplemented, "notification not configured")
		return
	}
	if body.Target == "" {
		body.Target = "demo-device-token"
	}
	note := notification.NewNotification(body.Target, notification.ChannelPush, body.Body)
	note.Title = body.Title
	if err := s.Notification.Notify(req.Context(), &note); err != nil {
		writeError(w, http.StatusInternalServerError, "notification error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sent": true})
}

type webhookRegisterRequest struct {
	Event  string `json:"event"`
	Target string `json:"target"`
	Secret string `json:"secret"`
}

// webhookRegister subscribes target to event.
func (s *Server) webhookRegister(w http.ResponseWriter, req *http.Request) {
	var body webhookRegisterRequest
	if err := decodeJSON(req, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if s.Webhook == nil {
		writeError(w, http.StatusNotImplemented, "webhook not configured")
		return
	}
	if body.Event == "" {
		body.Event = "demo.event"
	}
	if body.Target == "" {
		body.Target = "https://example.com/webhook"
	}
	if err := s.Webhook.Register(req.Context(), body.Event, body.Target, body.Secret); err != nil {
		writeError(w, http.StatusInternalServerError, "webhook error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"registered": body.Event})
}

type workflowStartRequest struct {
	Name  string `json:"name"`
	Input any    `json:"input"`
}

// RegisterDemoWorkflow registers the "demo-workflow" echo step when wf
// supports host-registered steps, so POST /demo/workflow/start works with
// zero setup. Other engines are left untouched. It asserts the
// workflow.StepRegistrar interface rather than a concrete adapter type;
// the container stays the sole resolver.
func RegisterDemoWorkflow(wf workflow.Workflow) {
	reg, ok := wf.(workflow.StepRegistrar)
	if !ok || reg == nil {
		return
	}
	reg.RegisterStep("demo-workflow", func(_ context.Context, input any) (any, error) {
		return input, nil
	})
}

// workflowStart begins a run of the named in-memory workflow.
func (s *Server) workflowStart(w http.ResponseWriter, req *http.Request) {
	var body workflowStartRequest
	if err := decodeJSON(req, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if s.Workflow == nil {
		writeError(w, http.StatusNotImplemented, "workflow not configured")
		return
	}
	if body.Name == "" {
		body.Name = "demo-workflow"
	}
	runID, err := s.Workflow.Start(req.Context(), body.Name, body.Input, "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "workflow error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run_id": runID})
}

type documentRenderRequest struct {
	Format string `json:"format"`
	Source string `json:"source"`
}

// documentRender renders source into format. Without a local browser it
// answers 502; the server stays up either way.
func (s *Server) documentRender(w http.ResponseWriter, req *http.Request) {
	var body documentRenderRequest
	if err := decodeJSON(req, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if s.Document == nil {
		writeError(w, http.StatusNotImplemented, "document not configured")
		return
	}
	format := document.OutputFormat(body.Format)
	if body.Source == "" {
		body.Source = "<h1>demo</h1>"
	}
	out, err := s.Document.Render(req.Context(), []byte(body.Source), format)
	if err != nil {
		writeError(w, http.StatusBadGateway, "document error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"bytes": len(out)})
}

// tenantDemo resolves the single-tenant id.
func (s *Server) tenantDemo(w http.ResponseWriter, req *http.Request) {
	if s.Tenant == nil {
		writeError(w, http.StatusNotImplemented, "tenant not configured")
		return
	}
	id, err := s.Tenant.Resolve(req.Context(), map[string]string{"host": req.Host})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "tenant error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tenant": id})
}

// observabilityDemo emits one counter and opens one span.
func (s *Server) observabilityDemo(w http.ResponseWriter, req *http.Request) {
	if s.Observability == nil {
		writeError(w, http.StatusNotImplemented, "observability not configured")
		return
	}
	ctx := req.Context()
	meter := s.Observability.Meter("demoapp")
	if err := meter.Counter(ctx, "demo.requests", 1); err != nil {
		writeError(w, http.StatusInternalServerError, "observability error")
		return
	}
	_, span := s.Observability.Tracer("demoapp").Start(ctx, "demo")
	span.End()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// dispatchJob enqueues a registered job by name.
func (s *Server) dispatchJob(w http.ResponseWriter, req *http.Request) {
	if s.Job == nil {
		writeError(w, http.StatusNotImplemented, "job not configured")
		return
	}
	name := router.Param(req, "name")
	var args map[string]any
	_ = decodeJSON(req, &args)
	if err := s.Job.Dispatch(req.Context(), name, args); err != nil {
		writeError(w, http.StatusInternalServerError, "dispatch failed")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"dispatched": name})
}
