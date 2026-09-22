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
	Password string `json:"password"`
	Phone    string `json:"phone"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// handleRegister creates a user with an argon2-hashed password.
// Duplicate emails conflict with 409. An optional phone is encrypted via
// crypto before storage.
func (a *API) handleRegister(w http.ResponseWriter, req *http.Request) {
	var in registerRequest
	if err := decodeJSON(req, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	in.Email = strings.TrimSpace(in.Email)
	if in.Email == "" || !strings.Contains(in.Email, "@") {
		writeError(w, http.StatusBadRequest, "invalid email")
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
		`INSERT INTO users (id, email, password_hash, created_at) VALUES (?, ?, ?, ?)`,
		id, in.Email, hash, now,
	); err != nil {
		if strings.Contains(strings.ToUpper(err.Error()), "UNIQUE") {
			writeError(w, http.StatusConflict, "email already registered")
			return
		}
		writeError(w, http.StatusInternalServerError, "create failed")
		return
	}

	if strings.TrimSpace(in.Phone) != "" && a.Crypto != nil {
		cipher, err := a.Crypto.Encrypt(ctx, []byte(strings.TrimSpace(in.Phone)))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "phone encrypt failed")
			return
		}
		if err := EnsureExtraTables(ctx, a.DB); err != nil {
			writeError(w, http.StatusInternalServerError, "phone store failed")
			return
		}
		if _, err := a.DB.Exec(ctx,
			`INSERT INTO guest_phones (user_id, cipher) VALUES (?, ?)`,
			id, cipher,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "phone store failed")
			return
		}
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

// handleMe returns the caller's profile with the decrypted phone and tenant.
func (a *API) handleMe(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	sub := subject(req)
	rows, err := a.DB.Query(ctx, `SELECT id, email FROM users WHERE id = ?`, sub)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	var id, email string
	if rows.Next() {
		if err := rows.Scan(&id, &email); err != nil {
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

	phone := ""
	if a.Crypto != nil {
		prows, err := a.DB.Query(ctx, `SELECT cipher FROM guest_phones WHERE user_id = ?`, sub)
		if err == nil {
			var cipher []byte
			if prows.Next() {
				if serr := prows.Scan(&cipher); serr == nil {
					if plain, derr := a.Crypto.Decrypt(ctx, cipher); derr == nil {
						phone = string(plain)
					}
				}
			}
			_ = prows.Close()
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"id": id, "email": email, "phone": phone, "tenant": a.tenantID(ctx, req),
	})
}
