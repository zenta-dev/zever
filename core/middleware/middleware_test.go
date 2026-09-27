package middleware

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
)

// recorderSpy implements http.ResponseWriter plus every optional writer
// interface middleware must forward: http.Flusher, http.Hijacker,
// http.Pusher, and io.ReaderFrom. Each optional method records that it was
// called so tests can prove forwarding happened.
type recorderSpy struct {
	header http.Header

	flushed  bool
	hijacked bool
	pushed   bool
	readFrom bool
}

func newRecorderSpy() *recorderSpy {
	return &recorderSpy{header: make(http.Header)}
}

func (s *recorderSpy) Header() http.Header { return s.header }

func (s *recorderSpy) Write(b []byte) (int, error) { return len(b), nil }

func (s *recorderSpy) WriteHeader(int) {}

func (s *recorderSpy) Flush() { s.flushed = true }

func (s *recorderSpy) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	s.hijacked = true

	return nil, nil, nil
}

func (s *recorderSpy) Push(string, *http.PushOptions) error {
	s.pushed = true

	return nil
}

func (s *recorderSpy) ReadFrom(io.Reader) (int64, error) {
	s.readFrom = true

	return 0, nil
}

func assertForwarded(t *testing.T, spy *recorderSpy) {
	t.Helper()

	if !spy.flushed {
		t.Error("Flush not forwarded to wrapped writer")
	}

	if !spy.hijacked {
		t.Error("Hijack not forwarded to wrapped writer")
	}

	if !spy.pushed {
		t.Error("Push not forwarded to wrapped writer")
	}

	if !spy.readFrom {
		t.Error("ReadFrom not forwarded to wrapped writer")
	}
}

// bareWriter implements only http.ResponseWriter — none of the optional
// interfaces (Flusher/Hijacker/Pusher/ReaderFrom).
type bareWriter struct {
	header http.Header
	status int
}

func (b *bareWriter) Header() http.Header { return b.header }

func (b *bareWriter) WriteHeader(status int) { b.status = status }

func (b *bareWriter) Write(p []byte) (int, error) { return len(p), nil }

func TestNewStatusRecorderIsIdempotent(t *testing.T) {
	t.Parallel()

	inner := &bareWriter{header: make(http.Header)}
	rec1 := newStatusRecorder(inner)
	rec2 := newStatusRecorder(rec1)

	if rec2 != rec1 {
		t.Fatal("newStatusRecorder must return the same *statusRecorder when given one, not wrap it again")
	}
}

func TestStatusRecorderFirstWriteWins(t *testing.T) {
	t.Parallel()

	inner := &bareWriter{header: make(http.Header)}
	rec := newStatusRecorder(inner)

	rec.WriteHeader(http.StatusCreated)
	rec.WriteHeader(http.StatusTeapot)

	if rec.status != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.status, http.StatusCreated)
	}
}

func TestStatusRecorderImplicit200OnWrite(t *testing.T) {
	t.Parallel()

	inner := &bareWriter{header: make(http.Header)}
	rec := newStatusRecorder(inner)

	if rec.status != http.StatusOK {
		t.Fatalf("initial status = %d, want %d", rec.status, http.StatusOK)
	}

	if _, err := rec.Write([]byte("hi")); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	if !rec.wroteHeader {
		t.Fatal("wroteHeader not recorded after Write")
	}

	if rec.status != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.status, http.StatusOK)
	}
}

func TestStatusRecorderWriteMarksHeader(t *testing.T) {
	t.Parallel()

	inner := &bareWriter{header: make(http.Header)}
	rec := newStatusRecorder(inner)

	if rec.wroteHeader {
		t.Fatal("wroteHeader must start false")
	}

	if _, err := rec.Write([]byte("x")); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	if !rec.wroteHeader {
		t.Fatal("wroteHeader not recorded after Write")
	}
}

func TestStatusRecorderForwardsOptionalInterfaces(t *testing.T) {
	t.Parallel()

	spy := newRecorderSpy()
	rec := newStatusRecorder(spy)

	var boxed any = rec

	fl, ok := boxed.(http.Flusher)
	if !ok {
		t.Fatal("statusRecorder over a flusher must still satisfy http.Flusher")
	}

	fl.Flush()

	if !spy.flushed {
		t.Fatal("Flush not forwarded to wrapped writer")
	}

	hj, ok := boxed.(http.Hijacker)
	if !ok {
		t.Fatal("statusRecorder over a hijacker must still satisfy http.Hijacker")
	}

	if _, _, err := hj.Hijack(); err != nil {
		t.Fatalf("Hijack forwarded but returned error: %v", err)
	}

	if !spy.hijacked {
		t.Fatal("Hijack not forwarded to wrapped writer")
	}

	pu, ok := boxed.(http.Pusher)
	if !ok {
		t.Fatal("statusRecorder over a pusher must still satisfy http.Pusher")
	}

	if err := pu.Push("/asset.js", nil); err != nil {
		t.Fatalf("Push forwarded but returned error: %v", err)
	}

	if !spy.pushed {
		t.Fatal("Push not forwarded to wrapped writer")
	}

	rf, ok := boxed.(io.ReaderFrom)
	if !ok {
		t.Fatal("statusRecorder over a ReaderFrom must still satisfy io.ReaderFrom")
	}

	if _, err := rf.ReadFrom(strings.NewReader("x")); err != nil {
		t.Fatalf("ReadFrom forwarded but returned error: %v", err)
	}

	if !spy.readFrom {
		t.Fatal("ReadFrom not forwarded to wrapped writer")
	}

	assertForwarded(t, spy)
}

func TestStatusRecorderWithPlainWriterDoesNotPanic(t *testing.T) {
	t.Parallel()

	inner := &bareWriter{header: make(http.Header)}
	rec := newStatusRecorder(inner)

	rec.WriteHeader(http.StatusTeapot)

	if rec.status != http.StatusTeapot {
		t.Fatalf("status = %d, want %d", rec.status, http.StatusTeapot)
	}

	if !rec.wroteHeader {
		t.Fatal("wroteHeader not recorded")
	}

	rec.Flush()

	if _, _, err := rec.Hijack(); !errors.Is(err, http.ErrNotSupported) {
		t.Fatalf("Hijack err = %v, want %v", err, http.ErrNotSupported)
	}

	if err := rec.Push("/", nil); !errors.Is(err, http.ErrNotSupported) {
		t.Fatalf("Push err = %v, want %v", err, http.ErrNotSupported)
	}

	if _, err := rec.ReadFrom(strings.NewReader("x")); !errors.Is(err, http.ErrNotSupported) {
		t.Fatalf("ReadFrom err = %v, want %v", err, http.ErrNotSupported)
	}
}

func TestStatusRecorderUnwrapReturnsInner(t *testing.T) {
	t.Parallel()

	inner := &bareWriter{header: make(http.Header)}
	rec := newStatusRecorder(inner)

	if got := rec.Unwrap(); got != http.ResponseWriter(inner) {
		t.Fatal("Unwrap must return the wrapped http.ResponseWriter")
	}
}
