package jobs_test

import (
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/examples/bookings/internal/service/jobs"
)

// seedBookingRow inserts one booking with the given start/status directly.
func seedBookingRow(t *testing.T, database db.DB, id, start, status string) {
	t.Helper()

	ctx := t.Context()
	now := time.Now().UTC().Format(time.RFC3339)

	if _, err := database.Exec(ctx,
		`INSERT INTO bookings (id, space_id, guest_id, start_date, end_date, status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, "space-1", "guest-1", start, start, status, now); err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
}

// TestDueBookingsWindowAndParsing pins the due-soon filter: only confirmed
// bookings inside [now, now+ReminderWindow] whose start date parses as
// YYYY-MM-DD or RFC3339 are returned.
func TestDueBookingsWindowAndParsing(t *testing.T) {
	database, _, ctx := newTestDB(t)
	seedBooking(t, database)

	seedBookingRow(t, database, "booking-rfc", "2030-05-31T10:00:00Z", "confirmed")
	seedBookingRow(t, database, "booking-bad", "not-a-date", "confirmed")
	seedBookingRow(t, database, "booking-cancelled", "2030-05-31", "cancelled")
	seedBookingRow(t, database, "booking-past", "2020-01-01", "confirmed")

	now := time.Date(2030, 5, 30, 0, 0, 0, 0, time.UTC)

	due, err := jobs.DueBookings(ctx, database, now)
	if err != nil {
		t.Fatalf("DueBookings: %v", err)
	}

	got := make(map[string]bool, len(due))
	for _, b := range due {
		got[b.ID] = true
	}

	for _, want := range []string{"booking-1", "booking-rfc"} {
		if !got[want] {
			t.Errorf("DueBookings missing %q (got %v)", want, got)
		}
	}

	for _, skip := range []string{"booking-bad", "booking-cancelled", "booking-past"} {
		if got[skip] {
			t.Errorf("DueBookings included %q, want skipped", skip)
		}
	}
}

// TestDueBookingsNoneWhenWindowEmpty proves a now past every start yields no
// due bookings.
func TestDueBookingsNoneWhenWindowEmpty(t *testing.T) {
	database, _, ctx := newTestDB(t)
	seedBooking(t, database)

	due, err := jobs.DueBookings(ctx, database, time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("DueBookings: %v", err)
	}

	if len(due) != 0 {
		t.Fatalf("DueBookings = %d, want 0", len(due))
	}
}
