package redisclient

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

func TestNew_alwaysReturnsFreshClient(t *testing.T) {
	opts := Options{Addr: "localhost:6379"}

	first, err := New(opts)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = first.Close() })

	second, err := New(opts)
	if err != nil {
		t.Fatalf("New() second error = %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })

	if first == second {
		t.Error("New() with same opts returned same client, want independent instances")
	}
}

func TestNew_invalidAddr_returnsError(t *testing.T) {
	if _, err := New(Options{Addr: "redis://"}); err == nil {
		t.Fatal("New() = nil error, want missing-host error")
	}
}

func TestClose_nilClient_returnsNil(t *testing.T) {
	if err := Close(nil); err != nil {
		t.Errorf("Close(nil) error = %v, want nil", err)
	}
}

// TestClose_independentClients_dontAffectEachOther is the regression test
// for the removed package-level singleton: closing one client built by New
// must not reach into or tear down another client built by New, since each
// now owns its connection independently.
func TestClose_independentClients_dontAffectEachOther(t *testing.T) {
	first, err := New(Options{Addr: "localhost:6379"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	second, err := New(Options{Addr: "localhost:6379"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })

	if first == second {
		t.Fatal("New() returned the same client twice, want independent instances")
	}

	if err := Close(first); err != nil {
		t.Fatalf("Close(first) error = %v", err)
	}

	// Closing first must be a no-op with respect to second: second must
	// still be a distinct, live *goredis.Client (not nil'd out or shared).
	if second == nil {
		t.Fatal("second client is nil after closing first")
	}
}

type scriptConn struct {
	mu      sync.Mutex
	reads   [][]byte
	written bytes.Buffer
	closed  bool
	onClose error
}

func (c *scriptConn) Read(b []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.reads) == 0 {
		return 0, io.EOF
	}

	n := copy(b, c.reads[0])
	c.reads[0] = c.reads[0][n:]

	if len(c.reads[0]) == 0 {
		c.reads = c.reads[1:]
	}

	return n, nil
}

func (c *scriptConn) Write(b []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.written.Write(b)
}

func (c *scriptConn) wrote() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	return strings.ToLower(c.written.String())
}

func (c *scriptConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.closed = true

	return c.onClose
}

func (c *scriptConn) LocalAddr() net.Addr { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)} }

func (c *scriptConn) RemoteAddr() net.Addr             { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)} }
func (c *scriptConn) SetDeadline(time.Time) error      { return nil }
func (c *scriptConn) SetReadDeadline(time.Time) error  { return nil }
func (c *scriptConn) SetWriteDeadline(time.Time) error { return nil }

func pooledFailCloseClient(t *testing.T, closeErr error) *goredis.Client {
	t.Helper()

	conn := &scriptConn{
		reads: [][]byte{
			[]byte("-ERR unknown command 'hello'\r\n"),
			[]byte("+OK\r\n"),
			[]byte("+OK\r\n"),
			[]byte("+PONG\r\n"),
		},
		onClose: closeErr,
	}

	c := goredis.NewClient(&goredis.Options{
		Addr: "127.0.0.1:0",
		Dialer: func(context.Context, string, string) (net.Conn, error) {
			return conn, nil
		},
	})

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	if err := c.Ping(ctx).Err(); err != nil {
		t.Fatalf("Ping() setup error = %v", err)
	}

	wire := conn.wrote()
	if !strings.Contains(wire, "hello") {
		t.Errorf("handshake missing HELLO (wire = %q)", wire)
	}

	if !strings.Contains(wire, "ping") {
		t.Errorf("setup missing PING (wire = %q)", wire)
	}

	return c
}

func TestClose_wrapsClientError(t *testing.T) {
	sentinel := errors.New("close boom")
	bad := pooledFailCloseClient(t, sentinel)

	err := Close(bad)
	if err == nil {
		t.Fatal("Close() = nil, want close-client error")
	}

	if !errors.Is(err, ErrCloseClient) {
		t.Errorf("errors.Is(err, ErrCloseClient) = false (err = %v)", err)
	}

	if !errors.Is(err, sentinel) {
		t.Errorf("errors.Is(err, sentinel) = false (err = %v)", err)
	}
}
