package redis

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

func resetRedis(t *testing.T) {
	t.Helper()

	if err := Close(); err != nil {
		t.Fatalf("Close() setup error = %v", err)
	}

	t.Cleanup(func() {
		if err := Close(); err != nil {
			t.Errorf("Close() cleanup error = %v", err)
		}
	})
}

func TestNew_sameOptions_reusesClient(t *testing.T) {
	resetRedis(t)

	opts := Options{Addr: "localhost:6379"}

	first, err := New(opts)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	second, err := New(opts)
	if err != nil {
		t.Fatalf("New() second error = %v", err)
	}

	if first != second {
		t.Error("New() with same opts returned different client, want reused instance")
	}
}

func TestNew_whitespaceEquivalentOptions_reusesClient(t *testing.T) {
	resetRedis(t)

	first, err := New(Options{Addr: "localhost:6379", Password: "pw"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	second, err := New(Options{Addr: "  localhost:6379 ", Password: " pw "})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if first != second {
		t.Error("New() with whitespace-equivalent opts returned different client, want reuse")
	}
}

func TestNew_differentOptions_replacesClient(t *testing.T) {
	resetRedis(t)

	first, err := New(Options{Addr: "localhost:6379"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	second, err := New(Options{Addr: "localhost:6380"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if first == second {
		t.Error("New() with different opts returned same client, want replacement")
	}
}

func TestNew_invalidAddr_returnsError(t *testing.T) {
	resetRedis(t)

	if _, err := New(Options{Addr: "redis://"}); err == nil {
		t.Fatal("New() = nil error, want missing-host error")
	}
}

func TestClose_noInstance_returnsNil(t *testing.T) {
	resetRedis(t)

	if err := Close(); err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}
}

func TestClose_afterNew_resetsSingleton(t *testing.T) {
	resetRedis(t)

	first, err := New(Options{Addr: "localhost:6379"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err = Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	second, err := New(Options{Addr: "localhost:6379"})
	if err != nil {
		t.Fatalf("New() after Close error = %v", err)
	}

	if first == second {
		t.Error("New() after Close returned same client, want fresh instance")
	}
}

func TestNew_concurrentSameOptions_safe(t *testing.T) {
	resetRedis(t)

	opts := Options{Addr: "localhost:6379"}

	const workers = 20

	clients := make([]any, workers)
	errs := make([]error, workers)

	var wg sync.WaitGroup

	for i := range workers {
		wg.Add(1)

		go func() {
			defer wg.Done()
			c, err := New(opts)
			clients[i] = c
			errs[i] = err
		}()
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("New() worker %d error = %v", i, err)
		}
	}

	for i := 1; i < workers; i++ {
		if clients[i] != clients[0] {
			t.Fatalf("New() worker %d got different client, want single reused instance", i)
		}
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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

func TestNew_closePreviousError_wrapped(t *testing.T) {
	resetRedis(t)

	sentinel := errors.New("close boom")
	bad := pooledFailCloseClient(t, sentinel)

	instance = &Pool{client: bad, opt: Options{Addr: "old:6379"}}

	t.Cleanup(func() { instance = nil })

	_, err := New(Options{Addr: "new:6379"})
	if err == nil {
		t.Fatal("New() = nil, want close-client error")
	}

	if !errors.Is(err, ErrCloseClient) {
		t.Errorf("errors.Is(err, ErrCloseClient) = false (err = %v)", err)
	}

	if !errors.Is(err, sentinel) {
		t.Errorf("errors.Is(err, sentinel) = false (err = %v)", err)
	}
}

func TestClose_clientError_wrapped(t *testing.T) {
	resetRedis(t)

	sentinel := errors.New("close boom")
	bad := pooledFailCloseClient(t, sentinel)

	instance = &Pool{client: bad, opt: Options{Addr: "old:6379"}}

	err := Close()
	if err == nil {
		t.Fatal("Close() = nil, want close-client error")
	}

	if !errors.Is(err, ErrCloseClient) {
		t.Errorf("errors.Is(err, ErrCloseClient) = false (err = %v)", err)
	}

	if !errors.Is(err, sentinel) {
		t.Errorf("errors.Is(err, sentinel) = false (err = %v)", err)
	}

	if instance != nil {
		t.Error("instance != nil after Close, want reset")
	}
}
