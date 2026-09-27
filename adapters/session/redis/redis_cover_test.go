package redis

// White-box coverage tests: direct *store construction over dedicated
// miniredis servers or RESP fakes, so transport-loss branches are
// reachable without touching the shared internal/redis Pool singleton
// that TestMain wires up.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"

	"github.com/zenta-dev/zever/core/session"
)

// coverStore builds a store over a dedicated miniredis server (never the
// shared TestMain singleton). With live false the server is closed
// immediately, so every command fails at the transport layer and the
// redis: create set / get / save get / delete error branches fire.
func coverStore(t *testing.T, live bool) *store {
	t.Helper()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)

	c := goredis.NewClient(&goredis.Options{Addr: mr.Addr(), DisableIdentity: true})
	t.Cleanup(func() { _ = c.Close() })

	if !live {
		mr.Close()
	}

	return &store{client: c, prefix: "cov-", ttl: 15 * time.Minute}
}

// discardCoverCommand reads one RESP array (command) from r and drops it.
// The fake server replays canned replies, so command bytes are ignored.
func discardCoverCommand(r *bufio.Reader) error {
	hdr, err := r.ReadString('\n')
	if err != nil {
		return err
	}

	hdr = strings.TrimSuffix(strings.TrimSuffix(hdr, "\n"), "\r")

	if !strings.HasPrefix(hdr, "*") {
		return fmt.Errorf("cover: expected array header, got %q", hdr)
	}

	n, err := strconv.Atoi(hdr[1:])
	if err != nil {
		return fmt.Errorf("cover: bad array length %q: %w", hdr, err)
	}

	for i := 0; i < n; i++ {
		lh, err := r.ReadString('\n')
		if err != nil {
			return err
		}

		lh = strings.TrimSuffix(strings.TrimSuffix(lh, "\n"), "\r")

		if !strings.HasPrefix(lh, "$") {
			return fmt.Errorf("cover: expected bulk header, got %q", lh)
		}

		ln, err := strconv.Atoi(lh[1:])
		if err != nil {
			return fmt.Errorf("cover: bad bulk length %q: %w", lh, err)
		}

		if _, err := io.CopyN(io.Discard, r, int64(ln)+2); err != nil {
			return err
		}
	}

	return nil
}

// coverRESPClient builds a go-redis client over net.Pipe with canned RESP
// replies, one per command. A HELLO rejection is prepended so the
// handshake falls back to RESP2 without consuming script steps. The fake
// fails fast on unexpected extra commands instead of hanging.
func coverRESPClient(t *testing.T, replies ...string) *goredis.Client {
	t.Helper()

	c1, c2 := net.Pipe()

	script := append([]string{"-ERR unknown command 'HELLO'\r\n"}, replies...)
	done := make(chan struct{})

	go func() {
		defer close(done)
		defer func() { _ = c2.Close() }()

		r := bufio.NewReader(c2)

		for _, rep := range script {
			if err := discardCoverCommand(r); err != nil {
				return
			}

			if _, err := io.WriteString(c2, rep); err != nil {
				return
			}
		}

		for {
			if err := discardCoverCommand(r); err != nil {
				return
			}

			if _, err := io.WriteString(c2, "-ERR unexpected command\r\n"); err != nil {
				return
			}
		}
	}()

	c := goredis.NewClient(&goredis.Options{
		// Literal IP: Addr is resolved eagerly on NewClient and a
		// hostname would stall on DNS. The Dialer ignores it anyway.
		Addr:            "127.0.0.1:6379",
		Protocol:        2,
		DisableIdentity: true,
		Dialer:          func(context.Context, string, string) (net.Conn, error) { return c1, nil },
		DialerRetries:   1,
		MaxRetries:      -1,
		PoolSize:        1,
	})

	t.Cleanup(func() {
		_ = c.Close()
		_ = c1.Close()
		<-done
	})

	return c
}

func TestCoverRedactAddr(t *testing.T) {
	t.Parallel()

	cases := map[string]struct{ in, want string }{
		"password masked": {"redis://user:s3cret@h:6379", "redis://user:xxxxx@h:6379"},
		"bare user":       {"redis://user@h:6379", "redis://user:xxxxx@h:6379"},
		"plain address":   {"h:6379", "h:6379"},
		"unparsable":      {"://%z", "://%z"},
	}

	for name, tc := range cases {
		if got := redactAddr(tc.in); got != tc.want {
			t.Errorf("%s: redactAddr(%q) = %q, want %q", name, tc.in, got, tc.want)
		}
	}
}

func TestCoverUnixNanoFromNano(t *testing.T) {
	t.Parallel()

	if got := unixNano(time.Time{}); got != 0 {
		t.Errorf("unixNano(zero) = %d, want 0", got)
	}
	if !fromNano(0).IsZero() {
		t.Error("fromNano(0) nonzero, want zero")
	}

	ts := time.Now().Truncate(time.Microsecond).UTC()

	if got := unixNano(ts); got == 0 {
		t.Error("unixNano(live) = 0, want nonzero")
	}
	if got := fromNano(ts.UnixNano()); got.UTC().IsZero() {
		t.Error("fromNano(live) zero, want nonzero")
	}
}

func TestCoverCreateSetError(t *testing.T) {
	t.Parallel()

	st := coverStore(t, false)

	if _, err := st.Create(t.Context(), 0); err == nil || !strings.Contains(err.Error(), "redis: create set") {
		t.Fatalf("Create over dead server err = %v, want wrap \"redis: create set\"", err)
	}
}

func TestCoverGetError(t *testing.T) {
	t.Parallel()

	st := coverStore(t, false)

	if _, err := st.Get(t.Context(), session.NewID()); err == nil || !strings.Contains(err.Error(), "redis: get") {
		t.Fatalf("Get over dead server err = %v, want wrap \"redis: get\"", err)
	}
}

// TestCoverSaveTransportError drives Save against a dead server: the
// WATCH that opens the transaction fails before the read inside it ever
// runs, so only the outer "redis: save" wrap is guaranteed here (the
// inner "redis: save get" and "redis: save set" wraps are covered by the
// RESP-scripted tests below, which reach saveTx's body).
func TestCoverSaveTransportError(t *testing.T) {
	t.Parallel()

	st := coverStore(t, false)

	if err := st.Save(t.Context(), session.Session{ID: session.NewID()}); err == nil || !strings.Contains(err.Error(), "redis: save") {
		t.Fatalf("Save over dead server err = %v, want wrap \"redis: save\"", err)
	}
}

func TestCoverDeleteError(t *testing.T) {
	t.Parallel()

	st := coverStore(t, false)

	if err := st.Delete(t.Context(), session.NewID()); err == nil || !strings.Contains(err.Error(), "redis: delete") {
		t.Fatalf("Delete over dead server err = %v, want wrap \"redis: delete\"", err)
	}
}

// TestCoverSaveGetError drives Save's WATCH/GET step against a RESP fake
// that opens the transaction fine but fails the read: the error branch
// surfaces as "redis: save get".
func TestCoverSaveGetError(t *testing.T) {
	t.Parallel()

	st := &store{
		client: coverRESPClient(t, "+OK\r\n" /* WATCH */, "-ERR boom\r\n" /* GET */),
		prefix: "cov-", ttl: time.Minute,
	}

	if err := st.Save(t.Context(), session.Session{ID: session.NewID()}); err == nil || !strings.Contains(err.Error(), "redis: save get") {
		t.Fatalf("Save GET err = %v, want wrap \"redis: save get\"", err)
	}
}

// TestCoverSaveSetError drives Save's write path against a RESP fake that
// opens the transaction, reports a missing record (null GET), then fails
// the MULTI/EXEC write: the walkthrough error branch surfaces as
// "redis: save set".
func TestCoverSaveSetError(t *testing.T) {
	t.Parallel()

	st := &store{
		client: coverRESPClient(t,
			"+OK\r\n",             // WATCH
			"$-1\r\n",             // GET (missing)
			"+OK\r\n",             // MULTI
			"+QUEUED\r\n",         // SET (queued)
			"*1\r\n-ERR boom\r\n", // EXEC (SET failed inside the transaction)
		),
		prefix: "cov-", ttl: time.Minute,
	}

	if err := st.Save(t.Context(), session.Session{ID: session.NewID()}); err == nil || !strings.Contains(err.Error(), "redis: save set") {
		t.Fatalf("Save SET err = %v, want wrap \"redis: save set\"", err)
	}
}

// TestCoverSaveZeroExpiryExisting plants a live record with no absolute
// expiry and a nonzero CreatedAt, then Saves: the existing CreatedAt must
// be preserved and the no-expiry property kept (SET without EX) rather
// than re-applying the store default TTL.
func TestCoverSaveZeroExpiryExisting(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	st := coverStore(t, true)
	id := session.NewID()

	createdAt := time.Now().Add(-time.Hour).Truncate(time.Second).UTC()

	raw, err := json.Marshal(wireSession{
		CreatedAt: createdAt.UnixNano(),
		UpdatedAt: createdAt.UnixNano(),
	})
	if err != nil {
		t.Fatalf("marshal wire: %v", err)
	}

	setErr := st.client.Set(ctx, st.key(id), raw, 0).Err()
	if setErr != nil {
		t.Fatalf("plant record: %v", setErr)
	}

	saveErr := st.Save(ctx, session.Session{ID: id, Data: map[string]any{"k": "v"}})
	if saveErr != nil {
		t.Fatalf("Save: %v", saveErr)
	}

	got, err := st.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if !got.ExpiresAt.IsZero() {
		t.Errorf("ExpiresAt = %v, want zero (existing record had no expiry)", got.ExpiresAt)
	}
	if !got.CreatedAt.Equal(createdAt) {
		t.Errorf("CreatedAt = %v, want %v (preserved from existing record)", got.CreatedAt, createdAt)
	}
}
