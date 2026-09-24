package redis

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/zenta-dev/zever/idempotency"
)

// discardRespCommand reads one RESP array (command) from r and drops it.
// The fake server ignores command bytes and replies per script step.
func discardRespCommand(r *bufio.Reader) error {
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

// bulkReply encodes raw bytes as a RESP bulk string.
func bulkReply(b []byte) string {
	return "$" + strconv.Itoa(len(b)) + "\r\n" + string(b) + "\r\n"
}

// pipeClient builds a go-redis client over net.Pipe with canned RESP
// replies, one per command. A HELLO rejection is prepended so the
// handshake falls back to RESP2 without consuming script steps.
// The server fails fast on unexpected extra commands instead of hanging.
func pipeClient(t *testing.T, replies []string) *goredis.Client {
	t.Helper()

	c1, c2 := net.Pipe()

	script := append([]string{"-ERR unknown command 'HELLO'\r\n"}, replies...)
	done := make(chan struct{})

	go func() {
		defer close(done)
		defer func() { _ = c2.Close() }()

		r := bufio.NewReader(c2)

		for _, rep := range script {
			if err := discardRespCommand(r); err != nil {
				return
			}

			if _, err := io.WriteString(c2, rep); err != nil {
				return
			}
		}

		for {
			if err := discardRespCommand(r); err != nil {
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

// dialErrClient builds a go-redis client whose dials always fail.
func dialErrClient(t *testing.T) *goredis.Client {
	t.Helper()

	c := goredis.NewClient(&goredis.Options{
		Addr:            "127.0.0.1:6379",
		Protocol:        2,
		DisableIdentity: true,
		Dialer:          func(context.Context, string, string) (net.Conn, error) { return nil, errors.New("dial boom") },
		DialerRetries:   1,
		MaxRetries:      -1,
		PoolSize:        1,
	})

	t.Cleanup(func() { _ = c.Close() })

	return c
}

// whiteStore builds a store wired to the given client without touching
// the shared singleton.
func whiteStore(t *testing.T, c *goredis.Client) *store {
	t.Helper()

	return &store{client: c, prefix: "t:", ttl: time.Second}
}

func TestCover_NewDefaults(t *testing.T) {
	t.Parallel()

	s, err := New(testOptions())
	if err != nil {
		t.Fatalf("New err = %v, want nil", err)
	}

	t.Cleanup(func() { _ = s.Close() })

	st, ok := s.(*store)
	if !ok {
		t.Fatalf("New store type = %T, want *store", s)
	}

	if st.prefix != defaultPrefix {
		t.Fatalf("prefix = %q, want %q", st.prefix, defaultPrefix)
	}

	if st.ttl != idempotency.DefaultTTL {
		t.Fatalf("ttl = %v, want %v", st.ttl, idempotency.DefaultTTL)
	}
}

func TestCover_NewExplicit(t *testing.T) {
	t.Parallel()

	opts := testOptions()
	opts.TTL = time.Hour
	opts.Redis.Prefix = "cov-"

	s, err := New(opts)
	if err != nil {
		t.Fatalf("New err = %v, want nil", err)
	}

	t.Cleanup(func() { _ = s.Close() })

	st, ok := s.(*store)
	if !ok {
		t.Fatalf("New store type = %T, want *store", s)
	}

	if st.prefix != "cov-" {
		t.Fatalf("prefix = %q, want %q", st.prefix, "cov-")
	}

	if st.ttl != time.Hour {
		t.Fatalf("ttl = %v, want %v", st.ttl, time.Hour)
	}
}

func TestCover_NewPingFail(t *testing.T) {
	t.Parallel()

	opts := testOptions()
	opts.Redis.Addr = "127.0.0.1:1"

	s, err := New(opts)
	if err == nil {
		if s != nil {
			_ = s.Close()
		}

		t.Fatal("New(dead) = nil error, want ping error")
	}
}

func TestCover_redactAddr(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		in   string
		want string
	}{
		"userinfo masked": {"redis://user:secret@localhost:6379", "redis://user:xxxxx@localhost:6379"},
		"plain":           {"localhost:6379", "localhost:6379"},
		"unparsable":      {"://bad", "://bad"},
		"whitespace":      {"  localhost:6379  ", "localhost:6379"},
	}

	for name, tc := range cases {
		if got := redactAddr(tc.in); got != tc.want {
			t.Errorf("redactAddr(%s: %q) = %q, want %q", name, tc.in, got, tc.want)
		}
	}
}

func TestCover_BeginCtxErr(t *testing.T) {
	t.Parallel()

	s := &store{prefix: "t:", ttl: time.Second}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := s.Begin(ctx, "k", idempotency.BeginOptions{}); err == nil {
		t.Fatal("Begin canceled ctx = nil error, want error")
	}
}

func TestCover_BeginSetNXErr(t *testing.T) {
	t.Parallel()

	s := whiteStore(t, dialErrClient(t))

	if _, err := s.Begin(t.Context(), freshKey(t), idempotency.BeginOptions{}); err == nil {
		t.Fatal("Begin dial err = nil error, want error")
	}
}

func TestCover_BeginSetNXProtoErr(t *testing.T) {
	t.Parallel()

	s := whiteStore(t, pipeClient(t, []string{"-ERR boom\r\n"}))

	if _, err := s.Begin(t.Context(), freshKey(t), idempotency.BeginOptions{}); err == nil {
		t.Fatal("Begin SetNX err = nil error, want error")
	}
}

func TestCover_BeginNilRetry(t *testing.T) {
	t.Parallel()

	s := whiteStore(t, pipeClient(t, []string{":0\r\n", "$-1\r\n", ":1\r\n"}))

	out, err := s.Begin(t.Context(), freshKey(t), idempotency.BeginOptions{Fingerprint: []byte("fp")})
	if err != nil {
		t.Fatalf("Begin err = %v, want nil", err)
	}

	if out.Replay {
		t.Fatal("Begin Replay = true, want false (retry winner)")
	}
}

func TestCover_BeginLoopExhaustion(t *testing.T) {
	t.Parallel()

	s := whiteStore(t, pipeClient(t, []string{":0\r\n", "$-1\r\n", ":0\r\n", "$-1\r\n", ":0\r\n", "$-1\r\n"}))

	_, err := s.Begin(t.Context(), freshKey(t), idempotency.BeginOptions{Fingerprint: []byte("fp")})
	if !errors.Is(err, idempotency.ErrInProgress) {
		t.Fatalf("Begin exhausted err = %v, want ErrInProgress", err)
	}
}

func TestCover_BeginGetErr(t *testing.T) {
	t.Parallel()

	s := whiteStore(t, pipeClient(t, []string{":0\r\n", "-ERR boom\r\n"}))

	if _, err := s.Begin(t.Context(), freshKey(t), idempotency.BeginOptions{}); err == nil {
		t.Fatal("Begin Get err = nil error, want error")
	}
}

func TestCover_BeginDecodeErr(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	s := newTestStore(t)
	key := freshKey(t)

	st, ok := s.(*store)
	if !ok {
		t.Fatalf("store type = %T, want *store", s)
	}

	if err := testMini.Set(st.prefix+key, "garbage-not-a-record"); err != nil {
		t.Fatalf("seed err = %v, want nil", err)
	}

	if _, err := s.Begin(ctx, key, idempotency.BeginOptions{}); !errors.Is(err, idempotency.ErrCorruptRecord) {
		t.Fatalf("Begin garbage err = %v, want ErrCorruptRecord", err)
	}
}

func TestCover_CompleteCtxErr(t *testing.T) {
	t.Parallel()

	s := &store{prefix: "t:", ttl: time.Second}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := s.Complete(ctx, "k", nil, []byte("r")); err == nil {
		t.Fatal("Complete canceled ctx = nil error, want error")
	}
}

func TestCover_CompleteGetErr(t *testing.T) {
	t.Parallel()

	s := whiteStore(t, pipeClient(t, []string{"-ERR boom\r\n"}))

	if err := s.Complete(t.Context(), freshKey(t), nil, []byte("r")); err == nil {
		t.Fatal("Complete Get err = nil error, want error")
	}
}

func TestCover_CompleteDecodeErr(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	s := newTestStore(t)
	key := freshKey(t)

	st, ok := s.(*store)
	if !ok {
		t.Fatalf("store type = %T, want *store", s)
	}

	if err := testMini.Set(st.prefix+key, "garbage-not-a-record"); err != nil {
		t.Fatalf("seed err = %v, want nil", err)
	}

	if err := s.Complete(ctx, key, nil, []byte("r")); !errors.Is(err, idempotency.ErrCorruptRecord) {
		t.Fatalf("Complete garbage err = %v, want ErrCorruptRecord", err)
	}
}

func TestCover_CompleteSetErrOnMissing(t *testing.T) {
	t.Parallel()

	s := whiteStore(t, pipeClient(t, []string{"$-1\r\n", "-ERR boom\r\n"}))

	if err := s.Complete(t.Context(), freshKey(t), nil, []byte("r")); err == nil {
		t.Fatal("Complete Set err = nil error, want error")
	}
}

func TestCover_CompleteFallbackSuccess(t *testing.T) {
	t.Parallel()

	fp := []byte("fp")
	done := bulkReply(encodeDone(fp, []byte("old")))
	s := whiteStore(t, pipeClient(t, []string{done, "$-1\r\n", "+OK\r\n"}))

	if err := s.Complete(t.Context(), freshKey(t), fp, []byte("new")); err != nil {
		t.Fatalf("Complete fallback err = %v, want nil", err)
	}
}

func TestCover_CompleteFallbackSetErr(t *testing.T) {
	t.Parallel()

	fp := []byte("fp")
	done := bulkReply(encodeDone(fp, []byte("old")))
	s := whiteStore(t, pipeClient(t, []string{done, "$-1\r\n", "-ERR boom\r\n"}))

	if err := s.Complete(t.Context(), freshKey(t), fp, []byte("new")); err == nil {
		t.Fatal("Complete fallback Set err = nil error, want error")
	}
}

func TestCover_CompleteSetArgsErr(t *testing.T) {
	t.Parallel()

	fp := []byte("fp")
	done := bulkReply(encodeDone(fp, []byte("old")))
	s := whiteStore(t, pipeClient(t, []string{done, "-ERR boom\r\n"}))

	if err := s.Complete(t.Context(), freshKey(t), fp, []byte("new")); err == nil {
		t.Fatal("Complete SetArgs err = nil error, want error")
	}
}

func TestCover_ForgetCtxErr(t *testing.T) {
	t.Parallel()

	s := &store{prefix: "t:", ttl: time.Second}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := s.Forget(ctx, "k"); err == nil {
		t.Fatal("Forget canceled ctx = nil error, want error")
	}
}

func TestCover_ForgetDelErr(t *testing.T) {
	t.Parallel()

	s := whiteStore(t, pipeClient(t, []string{"-ERR boom\r\n"}))

	if err := s.Forget(t.Context(), freshKey(t)); err == nil {
		t.Fatal("Forget Del err = nil error, want error")
	}
}
