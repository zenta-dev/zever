package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/zenta-dev/zever/core/router"
	genapp "github.com/zenta-dev/zever/examples/todo/generated/zenorm/orm/gen/app"
	"github.com/zenta-dev/zever/orm"
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
	// The orm sqlite path persists timestamps as RFC3339 text
	// (second precision), so truncate here so the create response
	// matches what a subsequent get/list reads back.
	now := time.Now().UTC().Truncate(time.Second)
	nowText := now.Format(time.RFC3339Nano)
	userID := subject(req)
	if err := orm.InsertInto(genapp.Notes).Values(
		orm.Set(genapp.NoteCols.ID, id),
		orm.Set(genapp.NoteCols.UserID, userID),
		orm.Set(genapp.NoteCols.Title, in.Title),
		orm.Set(genapp.NoteCols.Body, in.Body),
		orm.Set(genapp.NoteCols.Done, false),
		orm.Set(genapp.NoteCols.CreatedAt, now),
	).Exec(req.Context(), a.DB); err != nil {
		writeError(w, http.StatusInternalServerError, "create failed")
		return
	}

	writeJSON(w, http.StatusCreated, Note{
		ID: id, UserID: userID, Title: in.Title, Body: in.Body, CreatedAt: nowText,
	})
}

// handleListNotes lists the caller's notes.
func (a *API) handleListNotes(w http.ResponseWriter, req *http.Request) {
	rows, err := orm.From(genapp.Notes).
		Where(genapp.NoteCols.UserID.Eq(subject(req))).
		OrderBy(genapp.NoteCols.CreatedAt.Asc()).
		All(req.Context(), a.DB)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}

	notes := []Note{}
	for _, r := range rows {
		notes = append(notes, Note{
			ID:        r.ID,
			UserID:    r.UserID,
			Title:     r.Title,
			Body:      r.Body,
			Done:      r.Done,
			CreatedAt: r.CreatedAt.Format(time.RFC3339Nano),
		})
	}

	writeJSON(w, http.StatusOK, notes)
}

// scanNote scans one owned note; ok is false when no row matches id+owner.
func (a *API) scanOwnedNote(req *http.Request, id string) (Note, bool, error) {
	r, ok, err := orm.From(genapp.Notes).Where(orm.And(
		genapp.NoteCols.ID.Eq(id),
		genapp.NoteCols.UserID.Eq(subject(req)),
	)).First(req.Context(), a.DB)
	if err != nil {
		return Note{}, false, err
	}
	if !ok {
		return Note{}, false, nil
	}
	return Note{
		ID:        r.ID,
		UserID:    r.UserID,
		Title:     r.Title,
		Body:      r.Body,
		Done:      r.Done,
		CreatedAt: r.CreatedAt.Format(time.RFC3339Nano),
	}, true, nil
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

	done := n.Done
	if _, err := orm.UpdateTable(genapp.Notes).Where(orm.And(
		genapp.NoteCols.ID.Eq(id),
		genapp.NoteCols.UserID.Eq(subject(req)),
	)).Set(
		orm.Set(genapp.NoteCols.Title, n.Title),
		orm.Set(genapp.NoteCols.Body, n.Body),
		orm.Set(genapp.NoteCols.Done, done),
	).Exec(req.Context(), a.DB); err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}

	writeJSON(w, http.StatusOK, n)
}

// handleDeleteNote deletes an owned note, 404 for missing or foreign ids.
func (a *API) handleDeleteNote(w http.ResponseWriter, req *http.Request) {
	affected, err := orm.DeleteFrom(genapp.Notes).Where(orm.And(
		genapp.NoteCols.ID.Eq(router.Param(req, "id")),
		genapp.NoteCols.UserID.Eq(subject(req)),
	)).Exec(req.Context(), a.DB)
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
