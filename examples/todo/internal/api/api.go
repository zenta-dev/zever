package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/zenta-dev/zever/auth"
	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/password"
	"github.com/zenta-dev/zever/router"
)

// API holds the dependencies note and user handlers need.
type API struct {
	DB       db.DB
	Auth     auth.Auth
	Password password.Hasher
}

// New builds an API from resolved dependencies.
func New(database db.DB, authInst auth.Auth, hasher password.Hasher) *API {
	return &API{DB: database, Auth: authInst, Password: hasher}
}

// Routes registers register, login, and note CRUD routes onto r.
func (a *API) Routes(r router.Router) {
	r.Handle("POST", "/api/register", a.handleRegister)
	r.Handle("POST", "/api/login", a.handleLogin)
	r.Handle("GET", "/api/notes", a.withAuth(a.handleListNotes))
	r.Handle("POST", "/api/notes", a.withAuth(a.handleCreateNote))
	r.Handle("GET", "/api/notes/{id}", a.withAuth(a.handleGetNote))
	r.Handle("PATCH", "/api/notes/{id}", a.withAuth(a.handleUpdateNote))
	r.Handle("DELETE", "/api/notes/{id}", a.withAuth(a.handleDeleteNote))
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
