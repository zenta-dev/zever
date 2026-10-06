package cdc

import (
	"context"
	"encoding/binary"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pglogrepl"
	"github.com/jackc/pgx/v5/pgproto3"

	"github.com/zenta-dev/zever/core/outbox"
)

// fakeScript selects scripted server behavior per query stage.
type fakeScript struct {
	// slot is one of "ok", "duplicate", "error".
	slot string
	// lsn is one of "ok", "empty", "bad", "error".
	lsn string
	// repl is one of "ok", "error".
	repl string
	// stream holds backend messages sent after CopyBothResponse.
	stream []pgproto3.BackendMessage
}

// fakeRepl is a minimal PostgreSQL wire server speaking just enough of the
// simple query and copy-both protocols to drive the CDC consumer without a
// live database. It is hermetic and deterministic.
type fakeRepl struct {
	script  fakeScript
	ln      net.Listener
	updates chan []byte

	mu    sync.Mutex
	conns []net.Conn

	wg sync.WaitGroup
}

func mustFakeRepl(t *testing.T, script fakeScript) *fakeRepl {
	t.Helper()

	lc := net.ListenConfig{}

	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}

	f := &fakeRepl{script: script, ln: ln, updates: make(chan []byte, 64)}

	f.wg.Add(1)

	go f.serve()

	t.Cleanup(func() {
		_ = ln.Close()
	})

	return f
}

// dsn returns a client DSN pointed at the fake server.
func (f *fakeRepl) dsn() string {
	return "postgres://fake:fake@" + f.ln.Addr().String() + "/db?sslmode=disable"
}

func (f *fakeRepl) serve() {
	defer f.wg.Done()

	for {
		conn, err := f.ln.Accept()
		if err != nil {
			return
		}

		f.mu.Lock()
		f.conns = append(f.conns, conn)
		f.mu.Unlock()

		f.wg.Add(1)

		go func() {
			defer f.wg.Done()

			f.serveConn(conn)
		}()
	}
}

func (f *fakeRepl) serveConn(conn net.Conn) {
	defer func() { _ = conn.Close() }()

	be := pgproto3.NewBackend(conn, conn)

	if _, err := be.ReceiveStartupMessage(); err != nil {
		return
	}

	be.Send(&pgproto3.AuthenticationOk{})
	be.Send(&pgproto3.ParameterStatus{Name: "server_version", Value: "18.0"})
	be.Send(&pgproto3.ParameterStatus{Name: "client_encoding", Value: "UTF8"})
	be.Send(&pgproto3.BackendKeyData{ProcessID: 1, SecretKey: []byte{0, 0, 0, 1}})
	be.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})

	if err := be.Flush(); err != nil {
		return
	}

	for {
		msg, err := be.Receive()
		if err != nil {
			return
		}

		switch m := msg.(type) {
		case *pgproto3.Query:
			f.handleQuery(be, m.String)
		case *pgproto3.CopyData:
			select {
			case f.updates <- m.Data:
			default:
			}
		case *pgproto3.CopyDone, *pgproto3.Terminate:
			return
		}
	}
}

func (f *fakeRepl) handleQuery(be *pgproto3.Backend, sql string) {
	switch {
	case strings.HasPrefix(sql, "SELECT pg_create_logical_replication_slot"):
		switch f.script.slot {
		case "duplicate":
			be.Send(pgErrResponse("42710", "duplicate slot"))
		case "error":
			be.Send(pgErrResponse("42501", "permission denied"))
		default:
			be.Send(&pgproto3.CommandComplete{CommandTag: []byte("SELECT 1")})
		}

		be.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
		_ = be.Flush()
	case strings.Contains(sql, "confirmed_flush_lsn"):
		switch f.script.lsn {
		case "empty":
			be.Send(&pgproto3.CommandComplete{CommandTag: []byte("SELECT 0")})
			be.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
		case "bad":
			sendLSNRow(be, "not-a-lsn")
		case "error":
			be.Send(pgErrResponse("42501", "permission denied"))
			be.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
		default:
			sendLSNRow(be, "0/1")
		}

		_ = be.Flush()
	case strings.HasPrefix(sql, "START_REPLICATION"):
		if f.script.repl == "error" {
			be.Send(pgErrResponse("55000", "replication failed"))
			be.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
			_ = be.Flush()

			return
		}

		be.Send(&pgproto3.CopyBothResponse{OverallFormat: 0, ColumnFormatCodes: []uint16{}})

		for _, msg := range f.script.stream {
			be.Send(msg)
		}

		_ = be.Flush()
	default:
		be.Send(&pgproto3.CommandComplete{CommandTag: []byte("SELECT 1")})
		be.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
		_ = be.Flush()
	}
}

// drop closes every accepted server connection, simulating a network break
// while the consumer loop runs.
func (f *fakeRepl) drop() {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, conn := range f.conns {
		_ = conn.Close()
	}
}

// pgErrResponse builds a backend error message with code.
func pgErrResponse(code, msg string) *pgproto3.ErrorResponse {
	return &pgproto3.ErrorResponse{Severity: "ERROR", Code: code, Message: msg}
}

// sendLSNRow answers the slot LSN lookup with one text row.
func sendLSNRow(be *pgproto3.Backend, lsn string) {
	be.Send(&pgproto3.RowDescription{Fields: []pgproto3.FieldDescription{
		{Name: []byte("confirmed_flush_lsn"), DataTypeOID: 25, DataTypeSize: -1, TypeModifier: -1},
	}})
	be.Send(&pgproto3.DataRow{Values: [][]byte{[]byte(lsn)}})
	be.Send(&pgproto3.CommandComplete{CommandTag: []byte("SELECT 1")})
	be.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
}

// keepalivePayload builds a CopyData body carrying a primary keepalive.
func keepalivePayload(reply bool) []byte {
	body := make([]byte, 17)
	if reply {
		body[16] = 1
	}

	return append([]byte{pglogrepl.PrimaryKeepaliveMessageByteID}, body...)
}

// xlogEnvelopeBytes wraps WAL bytes in the XLogData body form handleXLogData
// takes (the replication type byte already stripped by the consumer loop).
func xlogEnvelopeBytes(wal []byte) []byte {
	return append(make([]byte, 24), wal...)
}

// xlogCopyData wraps WAL bytes in a CopyData server message.
func xlogCopyData(wal []byte) *pgproto3.CopyData {
	return &pgproto3.CopyData{Data: append([]byte{pglogrepl.XLogDataByteID}, xlogEnvelopeBytes(wal)...)}
}

// ldmWALBytes encodes a logical-decoding message with prefix and content.
func ldmWALBytes(prefix string, content []byte, lsn uint64) []byte {
	buf := []byte{byte(pglogrepl.MessageTypeMessage), 1}

	var tmp [8]byte

	binary.BigEndian.PutUint64(tmp[:], lsn)
	buf = append(buf, tmp[:]...)
	buf = append(buf, append([]byte(prefix), 0)...)

	var ln [4]byte

	binary.BigEndian.PutUint32(ln[:], uint32(len(content))) //nolint:gosec // test payloads are tiny
	buf = append(buf, ln[:]...)

	return append(buf, content...)
}

// mustCount waits until pub holds at least n messages.
func mustCount(t *testing.T, pub *stubPublisher, n int) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()

	for {
		if pub.count() >= n {
			return
		}

		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for %d messages", n)
		case <-ticker.C:
		}
	}
}

// mustUpdate waits for one client standby update.
func mustUpdate(t *testing.T, f *fakeRepl) []byte {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	select {
	case data := <-f.updates:
		return data
	case <-ctx.Done():
		t.Fatalf("timed out waiting for standby update")

		return nil
	}
}

// mustStart opens a store against f and starts its consumer.
func mustStart(t *testing.T, f *fakeRepl, pub *stubPublisher) outbox.Store {
	t.Helper()

	s, err := New(Options{
		Options: outbox.Options{
			DSN:         f.dsn(),
			Slot:        "fake_slot",
			Publication: "fake_pub",
			Prefix:      DefaultPrefix,
		},
		Publisher: pub,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() { _ = s.Close() })

	if err := s.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	return s
}

func TestReplEndToEnd(t *testing.T) {
	t.Parallel()

	msg := outbox.Message{ID: "repl-1", Topic: "orders", Payload: []byte("p")}
	payload, err := encodeMessage(msg)
	if err != nil {
		t.Fatalf("encodeMessage() error = %v", err)
	}

	f := mustFakeRepl(t, fakeScript{stream: []pgproto3.BackendMessage{
		&pgproto3.CopyData{Data: []byte{}},
		&pgproto3.CopyData{Data: keepalivePayload(false)},
		&pgproto3.CopyData{Data: keepalivePayload(true)},
		xlogCopyData(ldmWALBytes(DefaultPrefix, payload, 2)),
	}})

	pub := &stubPublisher{}
	s := mustStart(t, f, pub)

	mustCount(t, pub, 1)
	mustUpdate(t, f)

	if st := s.Status(); st.Processed != 1 {
		t.Errorf("Status().Processed = %d, want 1", st.Processed)
	}

	if err := s.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	if err := s.Start(t.Context()); err == nil {
		t.Error("Start(after Close) = nil error, want ErrClosed")
	}
}

func TestReplDuplicateSlotIgnored(t *testing.T) {
	t.Parallel()

	f := mustFakeRepl(t, fakeScript{slot: "duplicate"})
	pub := &stubPublisher{}
	mustStart(t, f, pub)
}

func TestReplSlotError(t *testing.T) {
	t.Parallel()

	f := mustFakeRepl(t, fakeScript{slot: "error"})
	pub := &stubPublisher{}

	s, err := New(Options{
		Options:   outbox.Options{DSN: f.dsn(), Slot: "s", Publication: "p", Prefix: "x"},
		Publisher: pub,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() { _ = s.Close() })

	err = s.Start(t.Context())
	if err == nil {
		t.Fatal("Start(slot error) = nil error, want error")
	}

	if !strings.Contains(err.Error(), "cdc: create slot") {
		t.Errorf("Start() error = %v, want containing cdc: create slot", err)
	}
}

func TestReplLSNNotFound(t *testing.T) {
	t.Parallel()

	f := mustFakeRepl(t, fakeScript{lsn: "empty"})
	pub := &stubPublisher{}

	s, err := New(Options{
		Options:   outbox.Options{DSN: f.dsn(), Slot: "s", Publication: "p", Prefix: "x"},
		Publisher: pub,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() { _ = s.Close() })

	if err := s.Start(t.Context()); err == nil {
		t.Fatal("Start(missing slot) = nil error, want error")
	} else if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Start() error = %v, want containing not found", err)
	}
}

func TestReplLSNBadValue(t *testing.T) {
	t.Parallel()

	f := mustFakeRepl(t, fakeScript{lsn: "bad"})
	pub := &stubPublisher{}

	s, err := New(Options{
		Options:   outbox.Options{DSN: f.dsn(), Slot: "s", Publication: "p", Prefix: "x"},
		Publisher: pub,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() { _ = s.Close() })

	if err := s.Start(t.Context()); err == nil {
		t.Fatal("Start(bad lsn) = nil error, want error")
	} else if !strings.Contains(err.Error(), "cdc: parse lsn") {
		t.Errorf("Start() error = %v, want containing cdc: parse lsn", err)
	}
}

func TestReplLSNQueryError(t *testing.T) {
	t.Parallel()

	f := mustFakeRepl(t, fakeScript{lsn: "error"})
	pub := &stubPublisher{}

	s, err := New(Options{
		Options:   outbox.Options{DSN: f.dsn(), Slot: "s", Publication: "p", Prefix: "x"},
		Publisher: pub,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() { _ = s.Close() })

	if err := s.Start(t.Context()); err == nil {
		t.Fatal("Start(lsn error) = nil error, want error")
	} else if !strings.Contains(err.Error(), "cdc: read slot lsn") {
		t.Errorf("Start() error = %v, want containing cdc: read slot lsn", err)
	}
}

func TestReplStartRejected(t *testing.T) {
	t.Parallel()

	f := mustFakeRepl(t, fakeScript{repl: "error"})
	pub := &stubPublisher{}

	s, err := New(Options{
		Options:   outbox.Options{DSN: f.dsn(), Slot: "s", Publication: "p", Prefix: "x"},
		Publisher: pub,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() { _ = s.Close() })

	if err := s.Start(t.Context()); err == nil {
		t.Fatal("Start(rejected) = nil error, want error")
	} else if !strings.Contains(err.Error(), "cdc: start replication") {
		t.Errorf("Start() error = %v, want containing cdc: start replication", err)
	}
}

func TestReplConnDropSetsRelayError(t *testing.T) {
	t.Parallel()

	f := mustFakeRepl(t, fakeScript{})
	pub := &stubPublisher{}
	s := mustStart(t, f, pub)

	f.drop()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()

	for {
		if st := s.Status(); st.LastError != "" {
			return
		}

		select {
		case <-ctx.Done():
			t.Fatal("timed out waiting for relay error")
		case <-ticker.C:
		}
	}
}

func TestReplLoopErrorResponse(t *testing.T) {
	t.Parallel()

	f := mustFakeRepl(t, fakeScript{stream: []pgproto3.BackendMessage{
		&pgproto3.ErrorResponse{Severity: "FATAL", Code: "57P01", Message: "terminated"},
	}})

	pub := &stubPublisher{}
	s := mustStart(t, f, pub)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()

	for {
		if st := s.Status(); st.LastError != "" {
			return
		}

		select {
		case <-ctx.Done():
			t.Fatal("timed out waiting for relay error")
		case <-ticker.C:
		}
	}
}
