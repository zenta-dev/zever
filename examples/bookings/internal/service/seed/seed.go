// Package seed populates the bookings database with development data.
package seed

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/zenta-dev/zever/db"
	genapp "github.com/zenta-dev/zever/examples/bookings/generated/zenorm/orm/gen/app"
	"github.com/zenta-dev/zever/orm"
	"github.com/zenta-dev/zever/password"

	// Blank import registers the argon2id adapter used to hash demo passwords.
	_ "github.com/zenta-dev/zever/password/argon2"
)

// Demo identities and content. IDs are fixed so re-runs find existing rows.
const (
	HostEmail   = "host@example.com"
	GuestEmail  = "guest@example.com"
	HostID      = "seed-host"
	GuestID     = "seed-guest"
	SpaceOneID  = "seed-space-loft"
	SpaceTwoID  = "seed-space-cabin"
	BookingID   = "seed-booking-one"
	DemoPassLen = 12
)

// Run seeds the database with development data: one demo host, one demo
// guest, two spaces, and one booking. Called once by db/seed/main.go after
// the database connection is confirmed reachable.
//
// Seeding is idempotent: existing rows are left untouched, so re-running
// this command is safe.
func Run(ctx context.Context, database db.DB) error {
	hostID, err := ensureUser(ctx, database, HostID, HostEmail)
	if err != nil {
		return err
	}
	guestID, err := ensureUser(ctx, database, GuestID, GuestEmail)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	spaces := []struct {
		id, title, desc string
		lat, lng        float64
		price           int64
	}{
		{SpaceOneID, "Sunny loft", "Bright downtown loft", 47.6, -122.3, 15000},
		{SpaceTwoID, "Forest cabin", "Quiet cabin in the woods", 47.7, -122.4, 20000},
	}
	for _, s := range spaces {
		if err := ensureSpace(ctx, database, s.id, hostID, s.title, s.desc, s.lat, s.lng, s.price, now); err != nil {
			return err
		}
	}

	return ensureBooking(ctx, database, BookingID, SpaceOneID, guestID, now)
}

func ensureUser(ctx context.Context, database db.DB, id, email string) (string, error) {
	existing, ok, err := orm.From(genapp.Users).Where(genapp.UserCols.Email.Eq(email)).First(ctx, database)
	if err != nil {
		return "", fmt.Errorf("[seed] lookup user: %w", err)
	}
	if ok {
		return existing.ID, nil
	}

	hash, err := password.Hash(ctx, uuid.NewString()[:DemoPassLen])
	if err != nil {
		return "", fmt.Errorf("[seed] hash password: %w", err)
	}

	if err := orm.InsertInto(genapp.Users).Values(
		orm.Set(genapp.UserCols.ID, id),
		orm.Set(genapp.UserCols.Email, email),
		orm.Set(genapp.UserCols.PasswordHash, hash),
		orm.Set(genapp.UserCols.CreatedAt, time.Now().UTC()),
	).Exec(ctx, database); err != nil {
		return "", fmt.Errorf("[seed] insert user: %w", err)
	}
	return id, nil
}

func ensureSpace(ctx context.Context, database db.DB, id, hostID, title, desc string, lat, lng float64, price int64, now time.Time) error {
	_, ok, err := orm.From(genapp.Spaces).Where(genapp.SpaceCols.ID.Eq(id)).First(ctx, database)
	if err != nil {
		return fmt.Errorf("[seed] lookup space: %w", err)
	}
	if ok {
		return nil
	}

	if err := orm.InsertInto(genapp.Spaces).Values(
		orm.Set(genapp.SpaceCols.ID, id),
		orm.Set(genapp.SpaceCols.HostID, hostID),
		orm.Set(genapp.SpaceCols.Title, title),
		orm.Set(genapp.SpaceCols.Description, desc),
		orm.Set(genapp.SpaceCols.Lat, lat),
		orm.Set(genapp.SpaceCols.Lng, lng),
		orm.Set(genapp.SpaceCols.PriceCents, price),
		orm.Set(genapp.SpaceCols.CreatedAt, now),
	).Exec(ctx, database); err != nil {
		return fmt.Errorf("[seed] insert space: %w", err)
	}
	return nil
}

func ensureBooking(ctx context.Context, database db.DB, id, spaceID, guestID string, now time.Time) error {
	_, ok, err := orm.From(genapp.Bookings).Where(genapp.BookingCols.ID.Eq(id)).First(ctx, database)
	if err != nil {
		return fmt.Errorf("[seed] lookup booking: %w", err)
	}
	if ok {
		return nil
	}

	start := now.Add(30 * 24 * time.Hour).Format("2006-01-02")
	end := now.Add(32 * 24 * time.Hour).Format("2006-01-02")
	if err := orm.InsertInto(genapp.Bookings).Values(
		orm.Set(genapp.BookingCols.ID, id),
		orm.Set(genapp.BookingCols.SpaceID, spaceID),
		orm.Set(genapp.BookingCols.GuestID, guestID),
		orm.Set(genapp.BookingCols.StartDate, start),
		orm.Set(genapp.BookingCols.EndDate, end),
		orm.Set(genapp.BookingCols.Status, "confirmed"),
		orm.Set(genapp.BookingCols.CreatedAt, now),
	).Exec(ctx, database); err != nil {
		return fmt.Errorf("[seed] insert booking: %w", err)
	}
	return nil
}
