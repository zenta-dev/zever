// Package seed populates the todo database with development data.
package seed

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/password"
	genapp "github.com/zenta-dev/zever/examples/todo/generated/zenorm/orm/gen/app"
	"github.com/zenta-dev/zever/orm"

	passwordargon2 "github.com/zenta-dev/zever/adapters/password/argon2"
)

// Demo credentials and content.
const (
	DemoEmail    = "demo@example.com"
	DemoPassword = "password123"
)

// Run seeds database with development data. Called once by db/seed/main.go
// after the database connection is confirmed reachable.
//
// Seeding is idempotent: existing rows are left untouched, so re-running this
// command is safe.
func Run(ctx context.Context, database db.DB) error {
	passwordargon2.Register()

	userID, err := ensureUser(ctx, database)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	notes := []struct {
		id        string
		title     string
		body      string
		done      bool
		createdAt time.Time
	}{
		{"seed-note-overdue", "Pay rent", "due yesterday", false, now.Add(-48 * time.Hour)},
		{"seed-note-fresh", "Buy milk", "2%", false, now},
		{"seed-note-done", "Read inbox", "done", true, now.Add(-48 * time.Hour)},
	}
	for _, n := range notes {
		if err := ensureNote(ctx, database, n.id, userID, n.title, n.body, n.done, n.createdAt); err != nil {
			return err
		}
	}
	return nil
}

func ensureUser(ctx context.Context, database db.DB) (string, error) {
	existing, ok, err := orm.From(genapp.Users).Where(genapp.UserCols.Email.Eq(DemoEmail)).First(ctx, database)
	if err != nil {
		return "", fmt.Errorf("[seed] lookup user: %w", err)
	}
	if ok {
		return existing.ID, nil
	}

	hash, err := password.Hash(ctx, DemoPassword)
	if err != nil {
		return "", fmt.Errorf("[seed] hash password: %w", err)
	}

	id := uuid.NewString()
	if err := orm.InsertInto(genapp.Users).Values(
		orm.Set(genapp.UserCols.ID, id),
		orm.Set(genapp.UserCols.Email, DemoEmail),
		orm.Set(genapp.UserCols.PasswordHash, hash),
		orm.Set(genapp.UserCols.CreatedAt, time.Now().UTC()),
	).Exec(ctx, database); err != nil {
		return "", fmt.Errorf("[seed] insert user: %w", err)
	}
	return id, nil
}

func ensureNote(ctx context.Context, database db.DB, id, userID, title, body string, done bool, createdAt time.Time) error {
	_, ok, err := orm.From(genapp.Notes).Where(genapp.NoteCols.ID.Eq(id)).First(ctx, database)
	if err != nil {
		return fmt.Errorf("[seed] lookup note: %w", err)
	}
	if ok {
		return nil
	}

	if err := orm.InsertInto(genapp.Notes).Values(
		orm.Set(genapp.NoteCols.ID, id),
		orm.Set(genapp.NoteCols.UserID, userID),
		orm.Set(genapp.NoteCols.Title, title),
		orm.Set(genapp.NoteCols.Body, body),
		orm.Set(genapp.NoteCols.Done, done),
		orm.Set(genapp.NoteCols.CreatedAt, createdAt),
	).Exec(ctx, database); err != nil {
		return fmt.Errorf("[seed] insert note: %w", err)
	}
	return nil
}
