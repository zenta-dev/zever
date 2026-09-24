// Package api holds the demo app's HTTP layer. Every battery is exercised
// through a /demo/* endpoint; the domain CRUD (/v1/*) runs on the generated
// zenorm tables. Handlers resolve batteries best-effort: a nil battery
// answers 501 instead of taking the server down.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/zenta-dev/zever/ai"
	"github.com/zenta-dev/zever/analytics"
	"github.com/zenta-dev/zever/auth"
	"github.com/zenta-dev/zever/billing"
	"github.com/zenta-dev/zever/cache"
	"github.com/zenta-dev/zever/crypto"
	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/document"
	"github.com/zenta-dev/zever/eventbus"
	"github.com/zenta-dev/zever/flag"
	"github.com/zenta-dev/zever/geo"
	"github.com/zenta-dev/zever/i18n"
	"github.com/zenta-dev/zever/idempotency"
	"github.com/zenta-dev/zever/job"
	"github.com/zenta-dev/zever/lock"
	"github.com/zenta-dev/zever/log"
	"github.com/zenta-dev/zever/mailer"
	"github.com/zenta-dev/zever/media"
	"github.com/zenta-dev/zever/notification"
	"github.com/zenta-dev/zever/observability"
	"github.com/zenta-dev/zever/password"
	"github.com/zenta-dev/zever/payment"
	"github.com/zenta-dev/zever/permission"
	"github.com/zenta-dev/zever/queue"
	"github.com/zenta-dev/zever/ratelimit"
	"github.com/zenta-dev/zever/router"
	"github.com/zenta-dev/zever/search"
	"github.com/zenta-dev/zever/secrets"
	"github.com/zenta-dev/zever/session"
	"github.com/zenta-dev/zever/storage"
	"github.com/zenta-dev/zever/tenant"
	"github.com/zenta-dev/zever/vectorstore"
	"github.com/zenta-dev/zever/webhook"
	"github.com/zenta-dev/zever/workflow"
)

// TokenTTL is the lifetime of issued JWTs.
const TokenTTL = time.Hour

// ErrUnauthenticated is returned when no subject is in the context.
var ErrUnauthenticated = errors.New("[api] not authenticated")

// Deps carries every resolved service the handlers need. Optional batteries
// stay nil when resolution failed; handlers answer 501 for those.
type Deps struct {
	DB            db.DB
	Auth          auth.Auth
	Password      password.Hasher
	Log           log.Logger
	Cache         cache.Cache
	Flag          flag.Flag
	Permission    permission.Checker
	RateLimit     ratelimit.Limiter
	Lock          lock.Locker
	Idempotency   idempotency.Store
	Session       session.Store
	Queue         queue.Queue
	Job           *job.Dispatcher
	EventBus      eventbus.EventBus
	Search        search.Search
	VectorStore   vectorstore.VectorStore
	Storage       storage.Storage
	Media         media.Media
	AI            ai.AI
	Geo           geo.Geo
	I18n          i18n.I18n
	Crypto        crypto.Crypto
	Secrets       secrets.Secrets
	Notification  notification.Notifier
	Mailer        mailer.Mailer
	Webhook       webhook.Webhook
	Workflow      workflow.Workflow
	Observability observability.Provider
	Analytics     analytics.Analytics
	Payment       payment.Payment
	Billing       billing.Billing
	Document      document.Document
	Tenant        tenant.Tenant
}

// Server holds all batteries used by handlers.
type Server struct {
	Deps
}

// New builds a Server from resolved dependencies.
func New(d Deps) *Server {
	return &Server{Deps: d}
}

// Routes registers auth, domain CRUD, battery showcase, and job dispatch
// routes onto r.
func (s *Server) Routes(r router.Router) {
	r.Handle("GET", "/health", s.health)

	r.Handle("POST", "/register", s.register)
	r.Handle("POST", "/login", s.login)
	r.Handle("GET", "/me", s.authenticated(s.me))

	r.Handle("GET", "/v1/products", s.listProducts)
	r.Handle("POST", "/v1/products", s.authenticated(s.createProduct))
	r.Handle("GET", "/v1/products/{id}", s.getProduct)
	r.Handle("GET", "/v1/orders", s.authenticated(s.listOrders))
	r.Handle("POST", "/v1/orders", s.authenticated(s.createOrder))
	r.Handle("GET", "/v1/orders/{id}", s.authenticated(s.getOrder))
	r.Handle("GET", "/v1/posts", s.listPosts)
	r.Handle("POST", "/v1/posts", s.authenticated(s.createPost))

	r.Handle("GET", "/demo/cache/{key}", s.cacheGet)
	r.Handle("POST", "/demo/cache/{key}", s.cacheSet)
	r.Handle("GET", "/demo/flag/{key}", s.flagGet)
	r.Handle("POST", "/demo/permission/check", s.permissionCheck)
	r.Handle("POST", "/demo/ratelimit/{key}", s.ratelimitCheck)
	r.Handle("POST", "/demo/lock/{key}", s.lockDemo)
	r.Handle("POST", "/demo/idempotency/{key}", s.idempotencyDemo)
	r.Handle("POST", "/demo/session", s.sessionCreate)
	r.Handle("GET", "/demo/session/{id}", s.sessionGet)
	r.Handle("POST", "/demo/queue/{topic}", s.queuePush)
	r.Handle("POST", "/demo/eventbus/publish", s.eventbusPublish)
	r.Handle("POST", "/demo/search/index", s.searchIndex)
	r.Handle("GET", "/demo/search", s.searchQuery)
	r.Handle("POST", "/demo/vector/upsert", s.vectorUpsert)
	r.Handle("POST", "/demo/vector/query", s.vectorQuery)
	r.Handle("POST", "/demo/storage/presign", s.storagePresign)
	r.Handle("POST", "/demo/media/upload", s.mediaUpload)
	r.Handle("POST", "/demo/ai/generate", s.aiGenerate)
	r.Handle("POST", "/demo/geo/geocode", s.geoGeocode)
	r.Handle("GET", "/demo/i18n/{locale}/{key}", s.i18nTranslate)
	r.Handle("POST", "/demo/crypto/encrypt", s.cryptoEncrypt)
	r.Handle("POST", "/demo/crypto/decrypt", s.cryptoDecrypt)
	r.Handle("GET", "/demo/secrets/{name}", s.secretsGet)
	r.Handle("POST", "/demo/notification", s.notificationSend)
	r.Handle("POST", "/demo/webhook/register", s.webhookRegister)
	r.Handle("POST", "/demo/workflow/start", s.workflowStart)
	r.Handle("POST", "/demo/document/render", s.documentRender)
	r.Handle("GET", "/demo/tenant", s.tenantDemo)
	r.Handle("GET", "/demo/observability", s.observabilityDemo)

	r.Handle("POST", "/demo/jobs/dispatch/{name}", s.dispatchJob)
}

type subjectKey struct{}

// authenticated verifies a Bearer JWT and stores the subject in the request
// context.
func (s *Server) authenticated(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		token := req.Header.Get("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(token, prefix) {
			writeError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		claims, err := s.Auth.Verify(req.Context(), strings.TrimPrefix(token, prefix))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		if claims.Subject == "" {
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		next(w, req.WithContext(context.WithValue(req.Context(), subjectKey{}, claims.Subject)))
	}
}

// subject returns the authenticated user id from the request context.
func subject(req *http.Request) string {
	sub, _ := req.Context().Value(subjectKey{}).(string)
	return sub
}

// writeJSON encodes v as JSON with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError encodes a message as a JSON error.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// decodeJSON decodes a JSON request body into v.
// maxRequestBodyBytes bounds every JSON request body this API decodes,
// closing off an unbounded-body DoS vector: without it, a client can send
// an arbitrarily large body and force full buffering before any validation
// runs.
const maxRequestBodyBytes = 1 << 20 // 1 MiB

func decodeJSON(req *http.Request, v any) error {
	defer func() { _ = req.Body.Close() }()
	body := http.MaxBytesReader(nil, req.Body, maxRequestBodyBytes)
	return json.NewDecoder(body).Decode(v)
}

// health reports liveness.
func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "service": "demoapp"})
}
