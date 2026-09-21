package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/zenta-dev/zever/router"
)

// Note is the JSON shape for notes.
type Note struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	Done      bool   `json:"done"`
	CreatedAt string `json:"created_at"`
}

type createNoteRequest struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type updateNoteRequest struct {
	Title *string `json:"title"`
	Body  *string `json:"body"`
	Done  *bool   `json:"done"`
}

// handleCreateNote creates a note owned by the caller.
func (a *API) handleCreateNote(w http.ResponseWriter, req *http.Request) {
	var in createNoteRequest
	if err := decodeJSON(req, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if strings.TrimSpace(in.Title) == "" {
		writeError(w, http.StatusBadRequest, "title required")
		return
	}

	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	userID := subject(req)
	if _, err := a.DB.Exec(req.Context(),
		`INSERT INTO notes (id, user_id, title, body, done, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		id, userID, in.Title, in.Body, 0, now,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "create failed")
		return
	}

	writeJSON(w, http.StatusCreated, Note{
		ID: id, UserID: userID, Title: in.Title, Body: in.Body, CreatedAt: now,
	})
}

// handleListNotes lists the caller's notes.
func (a *API) handleListNotes(w http.ResponseWriter, req *http.Request) {
	rows, err := a.DB.Query(req.Context(),
		`SELECT id, user_id, title, body, done, created_at FROM notes WHERE user_id = ? ORDER BY created_at ASC`,
		subject(req),
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}
	defer func() { _ = rows.Close() }()

	notes := []Note{}
	for rows.Next() {
		var n Note
		var done int64
		if scanErr := rows.Scan(&n.ID, &n.UserID, &n.Title, &n.Body, &done, &n.CreatedAt); scanErr != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		n.Done = done != 0
		notes = append(notes, n)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}

	writeJSON(w, http.StatusOK, notes)
}

// scanNote scans one owned note; ok is false when no row matches id+owner.
func (a *API) scanOwnedNote(req *http.Request, id string) (Note, bool, error) {
	rows, err := a.DB.Query(req.Context(),
		`SELECT id, user_id, title, body, done, created_at FROM notes WHERE id = ? AND user_id = ?`,
		id, subject(req),
	)
	if err != nil {
		return Note{}, false, err
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		_ = rows.Close()
		return Note{}, false, nil
	}
	var n Note
	var done int64
	if err := rows.Scan(&n.ID, &n.UserID, &n.Title, &n.Body, &done, &n.CreatedAt); err != nil {
		return Note{}, false, err
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return Note{}, false, err
	}
	n.Done = done != 0
	return n, true, nil
}

// handleGetNote returns one owned note, 404 for missing or foreign ids.
func (a *API) handleGetNote(w http.ResponseWriter, req *http.Request) {
	n, ok, err := a.scanOwnedNote(req, router.Param(req, "id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "get failed")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, n)
}

// handleUpdateNote patches title/body/done on an owned note.
func (a *API) handleUpdateNote(w http.ResponseWriter, req *http.Request) {
	id := router.Param(req, "id")
	n, ok, err := a.scanOwnedNote(req, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "get failed")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	var in updateNoteRequest
	if err := decodeJSON(req, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if in.Title != nil {
		n.Title = *in.Title
	}
	if in.Body != nil {
		n.Body = *in.Body
	}
	if in.Done != nil {
		n.Done = *in.Done
	}
	if strings.TrimSpace(n.Title) == "" {
		writeError(w, http.StatusBadRequest, "title required")
		return
	}

	done := 0
	if n.Done {
		done = 1
	}
	if _, err := a.DB.Exec(req.Context(),
		`UPDATE notes SET title = ?, body = ?, done = ? WHERE id = ? AND user_id = ?`,
		n.Title, n.Body, done, id, subject(req),
	); err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}

	writeJSON(w, http.StatusOK, n)
}

// handleDeleteNote deletes an owned note, 404 for missing or foreign ids.
func (a *API) handleDeleteNote(w http.ResponseWriter, req *http.Request) {
	affected, err := a.DB.Exec(req.Context(),
		`DELETE FROM notes WHERE id = ? AND user_id = ?`,
		router.Param(req, "id"), subject(req),
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	if affected == 0 {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
