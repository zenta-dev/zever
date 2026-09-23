package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	gen "github.com/zenta-dev/zever/examples/demoapp/generated/zenorm/orm/gen/app"
	"github.com/zenta-dev/zever/orm"
)

// minPasswordLen is the minimum accepted password length.
const minPasswordLen = 8

type registerRequest struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// register creates a user with an argon2-hashed password.
// Duplicate emails conflict with 409.
func (s *Server) register(w http.ResponseWriter, req *http.Request) {
	var in registerRequest
	if err := decodeJSON(req, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	in.Email = strings.TrimSpace(in.Email)
	in.Name = strings.TrimSpace(in.Name)
	if in.Email == "" || !strings.Contains(in.Email, "@") {
		writeError(w, http.StatusBadRequest, "invalid email")
		return
	}
	if in.Name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	if len(in.Password) < minPasswordLen {
		writeError(w, http.StatusBadRequest, "password too short")
		return
	}

	ctx := req.Context()
	_, exists, err := orm.From(gen.Users).Where(gen.UserCols.Email.Eq(in.Email)).First(ctx, s.DB)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if exists {
		writeError(w, http.StatusConflict, "email already registered")
		return
	}

	hash, err := s.Password.Hash(ctx, in.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "hash failed")
		return
	}

	id := uuid.NewString()
	now := time.Now().UTC()
	if err := orm.InsertInto(gen.Users).Values(
		orm.Set(gen.UserCols.ID, id),
		orm.Set(gen.UserCols.Email, in.Email),
		orm.Set(gen.UserCols.Name, in.Name),
		orm.Set(gen.UserCols.Role, "member"),
		orm.Set(gen.UserCols.PasswordHash, hash),
		orm.Set(gen.UserCols.CreatedAt, now),
	).Exec(ctx, s.DB); err != nil {
		if strings.Contains(strings.ToUpper(err.Error()), "UNIQUE") {
			writeError(w, http.StatusConflict, "email already registered")
			return
		}
		writeError(w, http.StatusInternalServerError, "create failed")
		return
	}

	if s.Analytics != nil {
		_ = s.Analytics.Track(ctx, "user.registered", map[string]any{"user_id": id})
	}
	if s.Cache != nil {
		_ = s.Cache.Set(ctx, "user:"+id, []byte(in.Email), 0)
	}

	writeJSON(w, http.StatusCreated, map[string]string{"id": id, "email": in.Email, "name": in.Name})
}

// login verifies credentials and issues a 1h JWT.
func (s *Server) login(w http.ResponseWriter, req *http.Request) {
	var in loginRequest
	if err := decodeJSON(req, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	ctx := req.Context()
	user, found, err := orm.From(gen.Users).Where(gen.UserCols.Email.Eq(strings.TrimSpace(in.Email))).First(ctx, s.DB)
	if err != nil || !found {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	ok, err := s.Password.Verify(ctx, user.PasswordHash, in.Password)
	if err != nil || !ok {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	token, err := s.Auth.Issue(ctx, user.ID, map[string]any{"email": user.Email, "role": user.Role}, TokenTTL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "issue failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"token": token.Value, "expires_at": token.ExpiresAt.UTC().Format(time.RFC3339)})
}

// me returns the caller's profile.
func (s *Server) me(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	user, found, err := orm.From(gen.Users).Where(gen.UserCols.ID.Eq(subject(req))).First(ctx, s.DB)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"id": user.ID, "email": user.Email, "name": user.Name, "role": user.Role})
}
