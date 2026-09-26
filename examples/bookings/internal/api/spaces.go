package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/zenta-dev/zever/core/geo"
	"github.com/zenta-dev/zever/core/router"
)

// Space is the JSON shape for spaces.
type Space struct {
	ID          string  `json:"id"`
	HostID      string  `json:"host_id"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Lat         float64 `json:"lat"`
	Lng         float64 `json:"lng"`
	PriceCents  int64   `json:"price_cents"`
	CreatedAt   string  `json:"created_at"`
}

type createSpaceRequest struct {
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Lat         float64 `json:"lat"`
	Lng         float64 `json:"lng"`
	PriceCents  int64   `json:"price_cents"`
}

type updateSpaceRequest struct {
	Title       *string  `json:"title"`
	Description *string  `json:"description"`
	Lat         *float64 `json:"lat"`
	Lng         *float64 `json:"lng"`
	PriceCents  *int64   `json:"price_cents"`
}

// handleCreateSpace creates a space owned by the caller as host.
func (a *API) handleCreateSpace(w http.ResponseWriter, req *http.Request) {
	var in createSpaceRequest
	if err := decodeJSON(req, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if strings.TrimSpace(in.Title) == "" || len(in.Title) > 200 {
		writeError(w, http.StatusBadRequest, "title must be 1..200 chars")
		return
	}
	if in.PriceCents < 0 {
		writeError(w, http.StatusBadRequest, "price_cents must be >= 0")
		return
	}
	if !geo.ValidCoord(in.Lat, in.Lng) {
		writeError(w, http.StatusBadRequest, "invalid coordinates")
		return
	}

	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	host := subject(req)
	if _, err := a.DB.Exec(req.Context(),
		`INSERT INTO spaces (id, host_id, title, description, lat, lng, price_cents, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, host, in.Title, in.Description, in.Lat, in.Lng, in.PriceCents, now,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "create failed")
		return
	}

	writeJSON(w, http.StatusCreated, Space{
		ID: id, HostID: host, Title: in.Title, Description: in.Description,
		Lat: in.Lat, Lng: in.Lng, PriceCents: in.PriceCents, CreatedAt: now,
	})
}

// scanSpace reads one space row by id.
func (a *API) scanSpace(req *http.Request, id string) (Space, bool, error) {
	rows, err := a.DB.Query(req.Context(),
		`SELECT id, host_id, title, description, lat, lng, price_cents, created_at FROM spaces WHERE id = ?`, id)
	if err != nil {
		return Space{}, false, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return Space{}, false, nil
	}
	var s Space
	if err := rows.Scan(&s.ID, &s.HostID, &s.Title, &s.Description, &s.Lat, &s.Lng, &s.PriceCents, &s.CreatedAt); err != nil {
		return Space{}, false, err
	}
	if err := rows.Err(); err != nil {
		return Space{}, false, err
	}
	return s, true, nil
}

// handleListSpaces lists all spaces.
func (a *API) handleListSpaces(w http.ResponseWriter, req *http.Request) {
	rows, err := a.DB.Query(req.Context(),
		`SELECT id, host_id, title, description, lat, lng, price_cents, created_at FROM spaces ORDER BY created_at ASC`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}
	defer func() { _ = rows.Close() }()

	out := []Space{}
	for rows.Next() {
		var s Space
		if err := rows.Scan(&s.ID, &s.HostID, &s.Title, &s.Description, &s.Lat, &s.Lng, &s.PriceCents, &s.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetSpace returns one space by id.
func (a *API) handleGetSpace(w http.ResponseWriter, req *http.Request) {
	s, ok, err := a.scanSpace(req, router.Param(req, "id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "get failed")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, s)
}

// handleUpdateSpace patches a space; only the host may modify (404 otherwise).
func (a *API) handleUpdateSpace(w http.ResponseWriter, req *http.Request) {
	id := router.Param(req, "id")
	s, ok, err := a.scanSpace(req, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "get failed")
		return
	}
	if !ok || s.HostID != subject(req) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	var in updateSpaceRequest
	if err := decodeJSON(req, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if in.Title != nil {
		s.Title = *in.Title
	}
	if in.Description != nil {
		s.Description = *in.Description
	}
	if in.Lat != nil {
		s.Lat = *in.Lat
	}
	if in.Lng != nil {
		s.Lng = *in.Lng
	}
	if in.PriceCents != nil {
		s.PriceCents = *in.PriceCents
	}
	if strings.TrimSpace(s.Title) == "" || len(s.Title) > 200 {
		writeError(w, http.StatusBadRequest, "title must be 1..200 chars")
		return
	}
	if s.PriceCents < 0 {
		writeError(w, http.StatusBadRequest, "price_cents must be >= 0")
		return
	}
	if !geo.ValidCoord(s.Lat, s.Lng) {
		writeError(w, http.StatusBadRequest, "invalid coordinates")
		return
	}
	if _, err := a.DB.Exec(req.Context(),
		`UPDATE spaces SET title = ?, description = ?, lat = ?, lng = ?, price_cents = ? WHERE id = ? AND host_id = ?`,
		s.Title, s.Description, s.Lat, s.Lng, s.PriceCents, id, subject(req)); err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	writeJSON(w, http.StatusOK, s)
}

// handleDeleteSpace deletes a space; only the host may delete (404 otherwise).
func (a *API) handleDeleteSpace(w http.ResponseWriter, req *http.Request) {
	affected, err := a.DB.Exec(req.Context(),
		`DELETE FROM spaces WHERE id = ? AND host_id = ?`, router.Param(req, "id"), subject(req))
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

// nearbyResult pairs a space with its distance in meters.
type nearbyResult struct {
	Space     Space   `json:"space"`
	DistanceM float64 `json:"distance_m"`
}

// handleNearby searches spaces near a point. ?q= geocodes via the static geo
// fixture; otherwise ?lat=&lng= is used. ?radius_km= defaults to 50.
func (a *API) handleNearby(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	q := req.URL.Query()

	var lat, lng float64
	if addr := strings.TrimSpace(q.Get("q")); addr != "" {
		if a.Geo == nil {
			writeError(w, http.StatusInternalServerError, "geo unavailable")
			return
		}
		locs, err := a.Geo.Geocode(ctx, addr)
		if err != nil || len(locs) == 0 {
			writeError(w, http.StatusNotFound, "address not found")
			return
		}
		lat, lng = locs[0].Lat, locs[0].Lng
	} else {
		var err error
		lat, err = strconv.ParseFloat(q.Get("lat"), 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "lat required")
			return
		}
		lng, err = strconv.ParseFloat(q.Get("lng"), 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "lng required")
			return
		}
		if !geo.ValidCoord(lat, lng) {
			writeError(w, http.StatusBadRequest, "invalid coordinates")
			return
		}
	}

	radiusKM := 50.0
	if raw := q.Get("radius_km"); raw != "" {
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil || v <= 0 {
			writeError(w, http.StatusBadRequest, "invalid radius_km")
			return
		}
		radiusKM = v
	}

	rows, err := a.DB.Query(ctx,
		`SELECT id, host_id, title, description, lat, lng, price_cents, created_at FROM spaces ORDER BY created_at ASC`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}
	defer func() { _ = rows.Close() }()

	out := []nearbyResult{}
	for rows.Next() {
		var s Space
		if err := rows.Scan(&s.ID, &s.HostID, &s.Title, &s.Description, &s.Lat, &s.Lng, &s.PriceCents, &s.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		var distM float64
		if a.Geo != nil {
			d, err := a.Geo.Distance(ctx, geo.Point{Lat: lat, Lng: lng}, geo.Point{Lat: s.Lat, Lng: s.Lng})
			if err != nil {
				continue
			}
			distM = d
		}
		if distM <= radiusKM*1000 {
			out = append(out, nearbyResult{Space: s, DistanceM: distM})
		}
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}
	writeJSON(w, http.StatusOK, out)
}
