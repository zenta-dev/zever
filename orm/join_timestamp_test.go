package orm

import (
	"context"
	"testing"
	"time"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/db/sqlite"
)

// tsUser/tsPost mirror the schema codegen shape for a timestamp-column
// entity and a belongs_to relation: a plain entity struct, a orm.Table/
// orm.Column set including a time.Time column, and a orm.NewRelation. They
// exist to regression-test two sqlite-specific behaviors the todo example
// first exposed after the legacy-ORM cutover:
//
// 1. a time.Time bound through orm.Insert/orm.Set is persisted as RFC3339
// text (encodeArgs at orm's execution boundary), so the codegen'd Scan
// -- which parses timestamp columns as RFC3339 -- round-trips; and
// 2. Join2/LeftJoin2 scan correctly even when either side has a timestamp
// column (the older collectRow scan ran each entity's Scan against
// still-empty values and failed on the eager timestamp parse).
type tsUser struct {
	ID        string
	Email     string
	CreatedAt time.Time
}

func (u *tsUser) Scan(row Row) error {
	var raw string
	if err := row.Scan(&u.ID, &u.Email, &raw); err != nil {
		return err
	}

	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return err
	}

	u.CreatedAt = t

	return nil
}

type tsPost struct {
	ID        string
	UserID    string
	Title     string
	CreatedAt time.Time
}

func (p *tsPost) Scan(row Row) error {
	var raw string
	if err := row.Scan(&p.ID, &p.UserID, &p.Title, &raw); err != nil {
		return err
	}

	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return err
	}

	p.CreatedAt = t

	return nil
}

var (
	tsUsers    = NewTable[tsUser]("ts_users", []string{"id", "email", "created_at"})
	tsUserID   = NewColumn[tsUser, string]("ts_users", "id")
	tsUserMail = NewColumn[tsUser, string]("ts_users", "email")
	tsUserAt   = NewColumn[tsUser, time.Time]("ts_users", "created_at")

	tsPosts     = NewTable[tsPost]("ts_posts", []string{"id", "user_id", "title", "created_at"})
	tsPostID    = NewColumn[tsPost, string]("ts_posts", "id")
	tsPostUser  = NewColumn[tsPost, string]("ts_posts", "user_id")
	tsPostTitle = NewColumn[tsPost, string]("ts_posts", "title")
	tsPostAt    = NewColumn[tsPost, time.Time]("ts_posts", "created_at")

	tsUserPostsRel = NewRelation[tsUser, tsPost]("id", "user_id", tsPosts)
)

// newTimestampDB lays out the ts_users/ts_posts tables in a fresh sqlite
// database (TEXT columns, matching the framework's own migration DDL) and
// seeds one user with one post, returning the connection.
func newTimestampDB(t *testing.T) (db.DB, time.Time) {
	t.Helper()

	conn, err := sqlite.New(db.Options{Path: t.TempDir() + "/ts.db"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	ctx := context.Background()

	for _, stmt := range []string{
		`CREATE TABLE ts_users (id TEXT, email TEXT, created_at TEXT)`,
		`CREATE TABLE ts_posts (id TEXT, user_id TEXT, title TEXT, created_at TEXT)`,
	} {
		if _, err := conn.Exec(ctx, stmt); err != nil {
			t.Fatalf("create table %q: %v", stmt, err)
		}
	}

	at := time.Now().UTC().Truncate(time.Second)

	if err := InsertInto(tsUsers).Values(
		Set(tsUserID, "u1"),
		Set(tsUserMail, "a@example.com"),
		Set(tsUserAt, at),
	).Exec(ctx, conn); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if err := InsertInto(tsPosts).Values(
		Set(tsPostID, "p1"),
		Set(tsPostUser, "u1"),
		Set(tsPostTitle, "hello"),
		Set(tsPostAt, at.Add(time.Minute)),
	).Exec(ctx, conn); err != nil {
		t.Fatalf("insert post: %v", err)
	}

	return conn, at
}

// TestTimestampRoundTripThroughSQLite proves a time.Time bound through
// InsertInto is persisted as RFC3339 text that the codegen'd Scan parses
// back -- the storage format the framework's migration DDL and orm's
// generated Scan both agree on.
func TestTimestampRoundTripThroughSQLite(t *testing.T) {
	conn, at := newTimestampDB(t)

	users, err := From(tsUsers).All(context.Background(), conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(users) != 1 {
		t.Fatalf("len(users) = %d, want 1", len(users))
	}

	if !users[0].CreatedAt.Equal(at) {
		t.Fatalf("CreatedAt = %v, want %v", users[0].CreatedAt, at)
	}
}

// TestJoinWithTimestampColumnScans proves Join2 and LeftJoin2 populate both
// sides of the joined row even when one side has a timestamp column -- the
// regression that the legacy collectRow-based scan broke (it ran the
// entity Scan against empty values, so the eager timestamp parse failed).
func TestJoinWithTimestampColumnScans(t *testing.T) {
	conn, _ := newTimestampDB(t)
	ctx := context.Background()

	inner, err := JoinOn(From(tsUsers), tsUserPostsRel, InnerJoin).All(ctx, conn)
	if err != nil {
		t.Fatalf("Join2.All: %v", err)
	}

	if len(inner) != 1 {
		t.Fatalf("join len = %d, want 1", len(inner))
	}

	if inner[0].A.Email != "a@example.com" || inner[0].B.Title != "hello" {
		t.Fatalf("join row = %+v", inner[0])
	}

	if inner[0].B.CreatedAt.IsZero() {
		t.Fatal("joined post CreatedAt was not scanned")
	}

	left, err := LeftJoinOn(From(tsUsers), tsUserPostsRel).All(ctx, conn)
	if err != nil {
		t.Fatalf("LeftJoin2.All: %v", err)
	}

	if len(left) != 1 {
		t.Fatalf("left join len = %d, want 1", len(left))
	}

	if post, ok := left[0].B.Get(); !ok || post.Title != "hello" {
		t.Fatalf("left join owner = %+v", left[0].B)
	}
}
