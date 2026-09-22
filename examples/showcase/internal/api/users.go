package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
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

// handleRegister creates a user with an argon2-hashed password.
// Duplicate emails conflict with 409.
func (a *API) handleRegister(w http.ResponseWriter, req *http.Request) {
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
	rows, err := a.DB.Query(ctx, `SELECT id FROM users WHERE email = ?`, in.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	exists := rows.Next()
	_ = rows.Close()
	if rowsErr := rows.Err(); rowsErr != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if exists {
		writeError(w, http.StatusConflict, "email already registered")
		return
	}

	hash, err := a.Password.Hash(ctx, in.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "hash failed")
		return
	}

	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := a.DB.Exec(ctx,
		`INSERT INTO users (id, email, name, role, password_hash, age, credit_cents, rating, score, verified, birthday, avatar, prefs, created_at) VALUES (?, ?, ?, 'member', ?, 0, 0, 0, 0, 0, '', '', '{}', ?)`,
		id, in.Email, in.Name, hash, now,
	); err != nil {
		if strings.Contains(strings.ToUpper(err.Error()), "UNIQUE") {
			writeError(w, http.StatusConflict, "email already registered")
			return
		}
		writeError(w, http.StatusInternalServerError, "create failed")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"id": id, "email": in.Email})
}

// handleLogin verifies credentials and issues a 1h JWT.
func (a *API) handleLogin(w http.ResponseWriter, req *http.Request) {
	var in loginRequest
	if err := decodeJSON(req, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	ctx := req.Context()
	rows, err := a.DB.Query(ctx, `SELECT id, password_hash FROM users WHERE email = ?`, strings.TrimSpace(in.Email))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	var id, hash string
	found := false
	if rows.Next() {
		if scanErr := rows.Scan(&id, &hash); scanErr == nil {
			found = true
		}
	}
	_ = rows.Close()
	if rowsErr := rows.Err(); rowsErr != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if !found {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	ok, err := a.Password.Verify(ctx, hash, in.Password)
	if err != nil || !ok {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	token, err := a.Auth.Issue(ctx, id, nil, time.Hour)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "issue failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"token": token.Value})
}

// handleMe returns the caller's profile.
func (a *API) handleMe(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	sub := subject(req)
	rows, err := a.DB.Query(ctx, `SELECT id, email, name FROM users WHERE id = ?`, sub)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	var id, email, name string
	if rows.Next() {
		if err := rows.Scan(&id, &email, &name); err != nil {
			_ = rows.Close()
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
	} else {
		_ = rows.Close()
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"id": id, "email": email, "name": name})
}
