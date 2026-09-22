package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Review is the JSON shape for reviews.
type Review struct {
	ID        string `json:"id"`
	SpaceID   string `json:"space_id"`
	GuestID   string `json:"guest_id"`
	Rating    int64  `json:"rating"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

type createReviewRequest struct {
	SpaceID string `json:"space_id"`
	Rating  int64  `json:"rating"`
	Body    string `json:"body"`
}

// handleCreateReview adds a guest review for a space.
func (a *API) handleCreateReview(w http.ResponseWriter, req *http.Request) {
	var in createReviewRequest
	if err := decodeJSON(req, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if strings.TrimSpace(in.SpaceID) == "" {
		writeError(w, http.StatusBadRequest, "space_id required")
		return
	}
	if in.Rating < 1 || in.Rating > 5 {
		writeError(w, http.StatusBadRequest, "rating must be 1..5")
		return
	}
	if strings.TrimSpace(in.Body) == "" {
		writeError(w, http.StatusBadRequest, "body required")
		return
	}

	ctx := req.Context()
	srows, err := a.DB.Query(ctx, `SELECT id FROM spaces WHERE id = ?`, in.SpaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "space lookup failed")
		return
	}
	exists := srows.Next()
	_ = srows.Close()
	if err := srows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "space lookup failed")
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "space not found")
		return
	}

	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	guest := subject(req)
	if _, err := a.DB.Exec(ctx,
		`INSERT INTO reviews (id, space_id, guest_id, rating, body, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		id, in.SpaceID, guest, in.Rating, in.Body, now,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "create failed")
		return
	}

	writeJSON(w, http.StatusCreated, Review{
		ID: id, SpaceID: in.SpaceID, GuestID: guest, Rating: in.Rating, Body: in.Body, CreatedAt: now,
	})
}

// handleListReviews lists reviews, optionally filtered by ?space_id=.
func (a *API) handleListReviews(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	spaceID := req.URL.Query().Get("space_id")
	query := `SELECT id, space_id, guest_id, rating, body, created_at FROM reviews ORDER BY created_at ASC`
	rows, err := a.DB.Query(ctx, query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}
	defer func() { _ = rows.Close() }()

	out := []Review{}
	for rows.Next() {
		var r Review
		if serr := rows.Scan(&r.ID, &r.SpaceID, &r.GuestID, &r.Rating, &r.Body, &r.CreatedAt); serr != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		if spaceID != "" && r.SpaceID != spaceID {
			continue
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}
	writeJSON(w, http.StatusOK, out)
}
