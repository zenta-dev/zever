package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// doRaw sends a raw body for malformed-payload cases.
func doRaw(t *testing.T, h http.Handler, method, path, token, raw, contentType string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(raw))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// TestUnauthenticatedRejected pins 401 for every protected route without a token.
func TestUnauthenticatedRejected(t *testing.T) {
	ts := newTestSetup(t)
	h := ts.handler

	cases := []struct{ method, path string }{
		{"GET", "/api/me"},
		{"POST", "/api/spaces"},
		{"GET", "/api/spaces"},
		{"GET", "/api/nearby?lat=41.8&lng=-87.6"},
		{"GET", "/api/spaces/space-1"},
		{"PATCH", "/api/spaces/space-1"},
		{"DELETE", "/api/spaces/space-1"},
		{"POST", "/api/bookings"},
		{"GET", "/api/bookings"},
		{"DELETE", "/api/bookings/b-1"},
		{"POST", "/api/reviews"},
		{"GET", "/api/reviews"},
	}
	for _, tc := range cases {
		rec := do(t, h, tc.method, tc.path, "", nil, nil)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s = %d, want 401 body %s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "missing bearer") && !strings.Contains(rec.Body.String(), "invalid token") {
			t.Errorf("%s %s body = %s, want auth error", tc.method, tc.path, rec.Body.String())
		}
	}
}

// TestMalformedJSON pins 400 for invalid JSON bodies on every mutating route.
func TestMalformedJSON(t *testing.T) {
	ts := newTestSetup(t)
	h := ts.handler

	register(t, h, "edge@example.com", "password123", "")
	token := login(t, h, "edge@example.com", "password123")

	rec := do(t, h, "POST", "/api/spaces", token, map[string]any{
		"title": "Edge", "lat": 41.8781, "lng": -87.6298,
	}, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("seed space: %d %s", rec.Code, rec.Body.String())
	}
	var space struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &space); err != nil {
		t.Fatalf("space id: %v", err)
	}

	cases := []struct{ method, path string }{
		{"POST", "/api/register"},
		{"POST", "/api/login"},
		{"POST", "/api/spaces"},
		{"PATCH", "/api/spaces/" + space.ID},
		{"POST", "/api/bookings"},
		{"POST", "/api/reviews"},
	}
	for _, tc := range cases {
		var tok string
		if tc.path != "/api/register" && tc.path != "/api/login" {
			tok = token
		}
		rec := doRaw(t, h, tc.method, tc.path, tok, "{bad-json", "application/json")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s %s = %d, want 400 body %s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "invalid JSON") {
			t.Errorf("%s %s body = %s, want invalid JSON", tc.method, tc.path, rec.Body.String())
		}
	}
}

// TestUnknownIDs pins 404 for missing space, booking, and review targets.
func TestUnknownIDs(t *testing.T) {
	ts := newTestSetup(t)
	h := ts.handler

	register(t, h, "u1@example.com", "password123", "")
	token := login(t, h, "u1@example.com", "password123")

	rec := do(t, h, "GET", "/api/spaces/does-not-exist", token, nil, nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("get unknown space = %d, want 404", rec.Code)
	}
	rec = do(t, h, "DELETE", "/api/spaces/does-not-exist", token, nil, nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("delete unknown space = %d, want 404", rec.Code)
	}
	rec = do(t, h, "DELETE", "/api/bookings/does-not-exist", token, nil, nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("cancel unknown booking = %d, want 404", rec.Code)
	}
	rec = do(t, h, "POST", "/api/bookings", token, map[string]string{
		"space_id": "does-not-exist", "start_date": "2026-10-01", "end_date": "2026-10-05",
	}, nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("book unknown space = %d, want 404 body %s", rec.Code, rec.Body.String())
	}
	rec = do(t, h, "POST", "/api/reviews", token, map[string]any{
		"space_id": "does-not-exist", "rating": 5, "body": "x",
	}, nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("review unknown space = %d, want 404", rec.Code)
	}
}

// TestValidationBoundaries pins 400 for empty, oversized, and out-of-range fields.
func TestValidationBoundaries(t *testing.T) {
	ts := newTestSetup(t)
	h := ts.handler

	register(t, h, "v@example.com", "password123", "")
	token := login(t, h, "v@example.com", "password123")

	tests := []struct {
		name   string
		method string
		path   string
		body   any
		want   string
	}{
		{"register bad email", "POST", "/api/register", map[string]string{"email": "nope", "password": "password123"}, "invalid email"},
		{"register short password", "POST", "/api/register", map[string]string{"email": "a@b.c", "password": "short"}, "password too short"},
		{"space empty title", "POST", "/api/spaces", map[string]any{"title": "  ", "lat": 41.8, "lng": -87.6}, "title must be"},
		{"space title too long", "POST", "/api/spaces", map[string]any{"title": strings.Repeat("x", 201), "lat": 41.8, "lng": -87.6}, "title must be"},
		{"space negative price", "POST", "/api/spaces", map[string]any{"title": "T", "lat": 41.8, "lng": -87.6, "price_cents": -1}, "price_cents"},
		{"space bad coords", "POST", "/api/spaces", map[string]any{"title": "T", "lat": 999, "lng": 999}, "invalid coordinates"},
		{"booking missing fields", "POST", "/api/bookings", map[string]string{"space_id": "x"}, "required"},
		{"booking end before start", "POST", "/api/bookings", map[string]string{"space_id": "x", "start_date": "2026-10-05", "end_date": "2026-10-01"}, "end_date must be after"},
		{"review bad rating low", "POST", "/api/reviews", map[string]any{"space_id": "x", "rating": 0, "body": "b"}, "rating must be"},
		{"review bad rating high", "POST", "/api/reviews", map[string]any{"space_id": "x", "rating": 6, "body": "b"}, "rating must be"},
		{"review empty body", "POST", "/api/reviews", map[string]any{"space_id": "x", "rating": 5, "body": "  "}, "body required"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tok := token
			if tc.path == "/api/register" {
				tok = ""
			}
			rec := do(t, h, tc.method, tc.path, tok, tc.body, nil)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("%s = %d, want 400 body %s", tc.name, rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tc.want) {
				t.Fatalf("%s body = %s, want %q", tc.name, rec.Body.String(), tc.want)
			}
		})
	}
}

// TestNearbyEdge pins 400/404 for missing coords, bad radius, unknown address.
func TestNearbyEdge(t *testing.T) {
	ts := newTestSetup(t)
	h := ts.handler

	register(t, h, "n@example.com", "password123", "")
	token := login(t, h, "n@example.com", "password123")

	rec := do(t, h, "GET", "/api/nearby", token, nil, nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("nearby no coords = %d, want 400", rec.Code)
	}
	rec = do(t, h, "GET", "/api/nearby?lat=abc&lng=-87.6", token, nil, nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("nearby bad lat = %d, want 400", rec.Code)
	}
	rec = do(t, h, "GET", "/api/nearby?lat=41.8&lng=-87.6&radius_km=0", token, nil, nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("nearby bad radius = %d, want 400", rec.Code)
	}
	rec = do(t, h, "GET", "/api/nearby?q=Nowhere-xyz-unknown", token, nil, nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("nearby unknown address = %d, want 404 body %s", rec.Code, rec.Body.String())
	}
}

// TestDoubleCancelIdempotent pins cancelling twice returns cancelled both times.
func TestDoubleCancelIdempotent(t *testing.T) {
	ts := newTestSetup(t)
	h := ts.handler

	register(t, h, "host@example.com", "password123", "")
	register(t, h, "guest@example.com", "password456", "")
	hostToken := login(t, h, "host@example.com", "password123")
	guestToken := login(t, h, "guest@example.com", "password456")

	rec := do(t, h, "POST", "/api/spaces", hostToken, map[string]any{
		"title": "Loft", "lat": 41.8781, "lng": -87.6298, "price_cents": 1000,
	}, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("space: %d %s", rec.Code, rec.Body.String())
	}
	var space struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &space); err != nil || space.ID == "" {
		t.Fatalf("space id: %v", err)
	}
	rec = do(t, h, "POST", "/api/bookings", guestToken, map[string]string{
		"space_id": space.ID, "start_date": "2026-12-01", "end_date": "2026-12-03",
	}, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("book: %d %s", rec.Code, rec.Body.String())
	}
	var booked struct {
		Booking struct {
			ID string `json:"id"`
		} `json:"booking"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &booked); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for i := 0; i < 2; i++ {
		rec = do(t, h, "DELETE", "/api/bookings/"+booked.Booking.ID, guestToken, nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("cancel[%d] = %d %s", i, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "cancelled") {
			t.Fatalf("cancel[%d] body = %s, want cancelled", i, rec.Body.String())
		}
	}
}
