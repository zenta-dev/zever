package api

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/zenta-dev/zever/apperror"
	"github.com/zenta-dev/zever/auth"
	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/flag"
	"github.com/zenta-dev/zever/i18n"
	"github.com/zenta-dev/zever/password"
	"github.com/zenta-dev/zever/permission"
	"github.com/zenta-dev/zever/queue"
	"github.com/zenta-dev/zever/ratelimit"
	"github.com/zenta-dev/zever/router"

	shopsvc "github.com/zenta-dev/zever/examples/showcase/internal/service/shop"
)

//go:embed locales/*.json
var localeFS embed.FS

// LocalesFS exposes the embedded catalogs for container wiring.
var LocalesFS = localeFS

// API holds the dependencies shop handlers need.
type API struct {
	DB         db.DB
	Auth       auth.Auth
	Password   password.Hasher
	RateLimit  ratelimit.Limiter
	Permission permission.Checker
	Queue      queue.Queue
	I18n       i18n.I18n
	Flag       flag.Flag
}

// New builds an API from resolved dependencies.
func New(
	database db.DB,
	authInst auth.Auth,
	hasher password.Hasher,
	limiter ratelimit.Limiter,
	perm permission.Checker,
	q queue.Queue,
	i18nInst i18n.I18n,
	flags flag.Flag,
) *API {
	return &API{
		DB: database, Auth: authInst, Password: hasher,
		RateLimit: limiter, Permission: perm, Queue: q,
		I18n: i18nInst, Flag: flags,
	}
}

// Routes registers auth, product, order, and review routes onto r.
func (a *API) Routes(r router.Router) {
	r.Handle("POST", "/api/register", a.handleRegister)
	r.Handle("POST", "/api/login", a.handleLogin)
	r.Handle("GET", "/api/me", a.withAuth(a.handleMe))

	r.Handle("POST", "/api/products", a.withAuth(a.handleCreateProduct))
	r.Handle("GET", "/api/products", a.withAuth(a.handleListProducts))
	r.Handle("GET", "/api/products/{id}", a.withAuth(a.handleGetProduct))
	r.Handle("PUT", "/api/products/{id}", a.withAuth(a.handleUpdateProduct))
	r.Handle("PATCH", "/api/products/{id}", a.withAuth(a.handlePatchProduct))
	r.Handle("DELETE", "/api/products/{id}", a.withAuth(a.handleDeleteProduct))

	r.Handle("POST", "/api/orders/checkout", a.withAuth(a.handleCheckout))
	r.Handle("GET", "/api/orders", a.withAuth(a.handleListOrders))
	r.Handle("GET", "/api/orders/{id}", a.withAuth(a.handleGetOrder))

	r.Handle("POST", "/api/reviews", a.withAuth(a.handleCreateReview))
	r.Handle("GET", "/api/reviews", a.withAuth(a.handleListReviews))
}

type ctxKey struct{}

// withAuth verifies a Bearer JWT and stores the subject in the request context.
func (a *API) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		token := req.Header.Get("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(token, prefix) {
			writeError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		claims, err := a.Auth.Verify(req.Context(), strings.TrimPrefix(token, prefix))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		if claims.Subject == "" {
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		next(w, req.WithContext(context.WithValue(req.Context(), ctxKey{}, claims.Subject)))
	}
}

// subject returns the authenticated user id from the request context.
func subject(req *http.Request) string {
	sub, _ := req.Context().Value(ctxKey{}).(string)
	return sub
}

// locale picks the request locale from ?locale= or Accept-Language.
func locale(req *http.Request) string {
	if q := req.URL.Query().Get("locale"); q != "" {
		return q
	}
	al := req.Header.Get("Accept-Language")
	if al == "" {
		return "en"
	}
	parts := strings.Split(al, ",")
	tag := strings.TrimSpace(strings.SplitN(parts[0], ";", 2)[0])
	if tag == "" {
		return "en"
	}
	return tag
}

// confirmedMessage renders the order confirmation string via i18n,
// falling back to a plain English message.
func (a *API) confirmedMessage(ctx context.Context, req *http.Request, orderID string, total int64) string {
	if a.I18n == nil {
		return "Order confirmed: " + orderID
	}
	msg, err := a.I18n.Translate(ctx, locale(req), "order.confirmed",
		map[string]string{"ID": orderID, "Total": strconv.FormatInt(total, 10)})
	if err != nil || msg == "" {
		return "Order confirmed: " + orderID
	}
	return msg
}

// checkoutVersion reports v2 when the new_checkout flag is on, else v1.
func (a *API) checkoutVersion(ctx context.Context) string {
	if a.Flag == nil {
		return "v1"
	}
	on, err := a.Flag.Bool(ctx, "new_checkout", false)
	if err != nil || !on {
		return "v1"
	}
	return "v2"
}

// shopService builds the shared shop impl over the API's infra, so /api
// handlers delegate to the same business logic as the generated wrappers.
func (a *API) shopService() *shopsvc.ShopServiceImpl {
	return shopsvc.NewShopServiceImpl(shopsvc.Deps{DB: a.DB, Queue: a.Queue})
}

// shopReq returns req with a context carrying the authenticated subject in
// the form the shop impl understands (see shopsvc.ContextWithSubject).
// Callers pass shopReq(req).Context() to the impl.
func shopReq(req *http.Request) *http.Request {
	return req.WithContext(shopsvc.ContextWithSubject(req.Context(), subject(req)))
}

// writeServiceError maps a service-layer error onto its HTTP status:
// apperror errors use their code mapping, anything else is a 500.
func writeServiceError(w http.ResponseWriter, err error) {
	var appErr *apperror.Error
	if errors.As(err, &appErr) {
		writeError(w, appErr.Code().HTTPStatus(), appErr.Message())
		return
	}
	writeError(w, http.StatusInternalServerError, "internal error")
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
