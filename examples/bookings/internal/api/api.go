package api

import (
	"context"
	"embed"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/zenta-dev/zever/auth"
	"github.com/zenta-dev/zever/billing"
	"github.com/zenta-dev/zever/crypto"
	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/flag"
	"github.com/zenta-dev/zever/geo"
	"github.com/zenta-dev/zever/i18n"
	"github.com/zenta-dev/zever/idempotency"
	"github.com/zenta-dev/zever/lock"
	"github.com/zenta-dev/zever/password"
	"github.com/zenta-dev/zever/payment"
	"github.com/zenta-dev/zever/permission"
	"github.com/zenta-dev/zever/queue"
	"github.com/zenta-dev/zever/ratelimit"
	"github.com/zenta-dev/zever/router"
	"github.com/zenta-dev/zever/tenant"
)

//go:embed locales/*.json
var localeFS embed.FS

// LocalesFS exposes the embedded confirmation catalogs for container wiring.
var LocalesFS = localeFS

// API holds the dependencies booking handlers need.
type API struct {
	DB          db.DB
	Auth        auth.Auth
	Password    password.Hasher
	Ratelimit   ratelimit.Limiter
	Idempotency idempotency.Store
	Lock        lock.Locker
	Payment     payment.Payment
	Billing     billing.Billing
	Queue       queue.Queue
	Crypto      crypto.Crypto
	Tenant      tenant.Tenant
	I18n        i18n.I18n
	Flag        flag.Flag
	Permission  permission.Checker
	Geo         geo.Geo
}

// New builds an API from resolved dependencies.
func New(
	database db.DB,
	authInst auth.Auth,
	hasher password.Hasher,
	limiter ratelimit.Limiter,
	idem idempotency.Store,
	locker lock.Locker,
	pay payment.Payment,
	bill billing.Billing,
	q queue.Queue,
	crypt crypto.Crypto,
	ten tenant.Tenant,
	i18nInst i18n.I18n,
	flags flag.Flag,
	perm permission.Checker,
	geoInst geo.Geo,
) *API {
	return &API{
		DB: database, Auth: authInst, Password: hasher,
		Ratelimit: limiter, Idempotency: idem, Lock: locker,
		Payment: pay, Billing: bill, Queue: q, Crypto: crypt,
		Tenant: ten, I18n: i18nInst, Flag: flags,
		Permission: perm, Geo: geoInst,
	}
}

// Routes registers auth, space, booking, and review routes onto r.
func (a *API) Routes(r router.Router) {
	r.Handle("POST", "/api/register", a.handleRegister)
	r.Handle("POST", "/api/login", a.handleLogin)
	r.Handle("GET", "/api/me", a.withAuth(a.handleMe))

	r.Handle("POST", "/api/spaces", a.withAuth(a.handleCreateSpace))
	r.Handle("GET", "/api/spaces", a.withAuth(a.handleListSpaces))
	r.Handle("GET", "/api/nearby", a.withAuth(a.handleNearby))
	r.Handle("GET", "/api/spaces/{id}", a.withAuth(a.handleGetSpace))
	r.Handle("PATCH", "/api/spaces/{id}", a.withAuth(a.handleUpdateSpace))
	r.Handle("DELETE", "/api/spaces/{id}", a.withAuth(a.handleDeleteSpace))

	r.Handle("POST", "/api/bookings", a.withAuth(a.handleCreateBooking))
	r.Handle("GET", "/api/bookings", a.withAuth(a.handleListBookings))
	r.Handle("DELETE", "/api/bookings/{id}", a.withAuth(a.handleCancelBooking))

	r.Handle("POST", "/api/reviews", a.withAuth(a.handleCreateReview))
	r.Handle("GET", "/api/reviews", a.withAuth(a.handleListReviews))
}

// EnsureExtraTables creates the hand-owned side tables the generated schema
// does not cover: encrypted guest phones and booking payment links.
func EnsureExtraTables(ctx context.Context, database db.DB) error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS guest_phones (user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE, cipher BLOB NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS booking_phones (booking_id TEXT PRIMARY KEY REFERENCES bookings(id) ON DELETE CASCADE, cipher BLOB NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS booking_payments (booking_id TEXT PRIMARY KEY REFERENCES bookings(id) ON DELETE CASCADE, payment_id TEXT NOT NULL, amount_cents INTEGER NOT NULL)`,
	} {
		if _, err := database.Exec(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
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

// tenantID resolves the tenant for the request, passing the X-Tenant header
// through as metadata. The single-tenant adapter returns its fixed ID.
func (a *API) tenantID(ctx context.Context, req *http.Request) string {
	if a.Tenant == nil {
		return ""
	}
	id, err := a.Tenant.Resolve(ctx, map[string]string{"tenant": req.Header.Get("X-Tenant")})
	if err != nil {
		return ""
	}
	return id
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

// confirmedMessage renders the booking confirmation string via i18n,
// falling back to a plain English message.
func (a *API) confirmedMessage(ctx context.Context, req *http.Request, spaceTitle string) string {
	if a.I18n == nil {
		return "Booking confirmed for " + spaceTitle
	}
	msg, err := a.I18n.Translate(ctx, locale(req), "booking.confirmed", map[string]string{"Space": spaceTitle})
	if err != nil || msg == "" {
		return "Booking confirmed for " + spaceTitle
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
func decodeJSON(req *http.Request, v any) error {
	defer func() { _ = req.Body.Close() }()
	return json.NewDecoder(req.Body).Decode(v)
}
