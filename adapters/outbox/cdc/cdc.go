package cdc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pglogrepl"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/outbox"
	"github.com/zenta-dev/zever/shared/retry"
)

// emitSQL publishes one outbox message as a transactional logical-replication
// message. transactional=true means the message is only replicated when the
// surrounding transaction commits, so a rollback drops the event.
const emitSQL = `SELECT pg_logical_emit_message(true, $1, $2::bytea)`

// store is a CDC-backed outbox.Store. It is safe for concurrent use.
type store struct {
	dsn         string
	prefix      string
	slot        string
	publication string
	publisher   outbox.Publisher
	retry       retry.Policy
	maxAttempts int
	sleep       sleepFunc
	recorder    outbox.Recorder

	// lastLSN is the replication position safe to acknowledge: it only
	// advances past messages that were published (or filtered out).
	lastLSN   atomic.Uint64
	processed atomic.Int64
	failed    atomic.Int64
	pending   atomic.Int64
	relayErr  atomic.Pointer[string]

	mu      sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{}
	conn    *pgconn.PgConn
	started bool
	closed  bool
}

// compile-time check that store satisfies the outbox.Store interface.
var _ outbox.Store = (*store)(nil)

// New creates a CDC-backed outbox store from o. The DSN must point at a
// PostgreSQL server with wal_level=logical. Publisher may be nil for
// record-only use; Start requires it. Empty Prefix/Slot/Publication resolve
// to the package defaults before validation.
func New(o Options) (outbox.Store, error) {
	if o.Prefix == "" {
		o.Prefix = DefaultPrefix
	}

	if o.Slot == "" {
		o.Slot = DefaultSlot
	}

	if o.Publication == "" {
		o.Publication = DefaultPublication
	}

	if err := o.Validate(); err != nil {
		return nil, err
	}

	maxAttempts := o.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = outbox.DefaultMaxAttempts
	}

	s := &store{
		dsn:         o.DSN,
		prefix:      o.Prefix,
		slot:        o.Slot,
		publication: o.Publication,
		publisher:   o.Publisher,
		retry:       o.Retry,
		maxAttempts: maxAttempts,
		sleep:       sleepCtx,
		recorder:    outbox.NewRecorder(o.Provider, string(outbox.CDC), ""),
	}

	return s, nil
}

// Record persists msg inside tx, the same transaction as the business write,
// by emitting it as a transactional logical-replication message. A nil tx
// fails with outbox.ErrTxRequired. The message is marshaled to its JSON wire
// shape so the consumer can decode it back into an outbox.Message.
func (s *store) Record(ctx context.Context, tx db.Tx, msg outbox.Message) error {
	if tx == nil {
		return fmt.Errorf("cdc: record: %w", outbox.ErrTxRequired)
	}

	if err := msg.Validate(); err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	payload, err := encodeMessage(msg)
	if err != nil {
		return fmt.Errorf("cdc: record %q: %w", msg.ID, err)
	}

	if _, err := tx.Exec(ctx, emitSQL, s.prefix, payload); err != nil {
		return fmt.Errorf("cdc: record %q: %w", msg.ID, err)
	}

	return nil
}

// Name returns the adapter name.
func (s *store) Name() string { return string(outbox.CDC) }

// Start connects a replication connection, ensures the logical replication
// slot exists, and launches the consumer loop. It is idempotent and returns
// an error when no Publisher is wired. On ctx cancel the loop stops.
func (s *store) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrClosed
	}

	if s.started {
		return nil
	}

	if s.publisher == nil {
		return fmt.Errorf("cdc: start: publisher is nil: %w", outbox.ErrInvalidOptions)
	}

	conn, err := s.connectReplication(ctx)
	if err != nil {
		return err
	}

	if slotErr := s.ensureSlot(ctx, conn); slotErr != nil {
		_ = conn.Close(ctx)

		return slotErr
	}

	lsn, err := s.readStartLSN(ctx, conn)
	if err != nil {
		_ = conn.Close(ctx)

		return err
	}

	err = pglogrepl.StartReplication(ctx, conn, s.slot, lsn, pglogrepl.StartReplicationOptions{
		Mode: pglogrepl.LogicalReplication,
		PluginArgs: []string{
			"proto_version '1'",
			fmt.Sprintf("publication_names '%s'", s.publication),
			"messages 'true'",
		},
	})
	if err != nil {
		_ = conn.Close(ctx)

		return fmt.Errorf("cdc: start replication: %w", err)
	}

	runCtx, cancel := context.WithCancel(ctx)

	s.cancel = cancel
	s.done = make(chan struct{})
	s.conn = conn
	s.started = true
	s.lastLSN.Store(uint64(lsn))

	go s.loop(runCtx, conn, s.done)

	return nil
}

// connectReplication opens a replication-mode connection by adding
// replication=database to the DSN runtime parameters.
func (s *store) connectReplication(ctx context.Context) (*pgconn.PgConn, error) {
	cfg, err := pgconn.ParseConfig(s.dsn)
	if err != nil {
		return nil, fmt.Errorf("cdc: parse dsn: %w", err)
	}

	cfg.RuntimeParams["replication"] = "database"

	conn, err := pgconn.ConnectConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("cdc: connect: %w", err)
	}

	return conn, nil
}

// ensureSlot creates the logical replication slot when missing, ignoring the
// duplicate_slot error for an existing one.
func (s *store) ensureSlot(ctx context.Context, conn *pgconn.PgConn) error {
	result := conn.Exec(ctx, `SELECT pg_create_logical_replication_slot(`+quoteLiteral(s.slot)+`, 'pgoutput')`)
	if _, err := result.ReadAll(); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "42710" {
			return nil
		}

		return fmt.Errorf("cdc: create slot: %w", err)
	}

	return nil
}

// readStartLSN returns the slot's confirmed_flush_lsn, the position the
// consumer resumes from.
func (s *store) readStartLSN(ctx context.Context, conn *pgconn.PgConn) (pglogrepl.LSN, error) {
	result := conn.Exec(ctx, `SELECT confirmed_flush_lsn FROM pg_replication_slots WHERE slot_name = `+quoteLiteral(s.slot))
	results, err := result.ReadAll()
	if err != nil {
		return 0, fmt.Errorf("cdc: read slot lsn: %w", err)
	}

	if len(results) != 1 || len(results[0].Rows) != 1 || len(results[0].Rows[0]) != 1 {
		return 0, fmt.Errorf("cdc: slot %q not found", s.slot)
	}

	var lsn pglogrepl.LSN
	if err := lsn.Scan(results[0].Rows[0][0]); err != nil {
		return 0, fmt.Errorf("cdc: parse lsn: %w", err)
	}

	return lsn, nil
}

// Close stops the consumer loop and closes the replication connection. It is
// idempotent.
func (s *store) Close() error {
	s.mu.Lock()

	if s.closed {
		s.mu.Unlock()

		return nil
	}

	s.closed = true

	cancel := s.cancel
	done := s.done
	conn := s.conn

	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	if done != nil {
		<-done
	}

	if conn != nil {
		return conn.Close(context.Background())
	}

	return nil
}

// Status reports consumer counters and the most recent error. Pending is
// always zero for CDC: the WAL is the queue, so a message is either published
// or failed while the consumer runs.
func (s *store) Status() outbox.Status {
	var st outbox.Status

	st.Pending = s.pending.Load()
	st.Processed = s.processed.Load()
	st.Failed = s.failed.Load()

	if ptr := s.relayErr.Load(); ptr != nil && *ptr != "" {
		st.LastError = *ptr
	}

	return st
}

// setRelayError records the most recent consumer error for Status.
func (s *store) setRelayError(ctx context.Context, err error) {
	if err == nil {
		return
	}

	str := err.Error()
	s.relayErr.Store(&str)
	s.recorder.RelayError(ctx)
}

// encodeMessage marshals msg to its JSON wire payload.
func encodeMessage(msg outbox.Message) ([]byte, error) {
	payload, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("cdc: encode message: %w", err)
	}

	return payload, nil
}

// decodeMessage unmarshals a JSON wire payload into msg.
func decodeMessage(payload []byte) (outbox.Message, error) {
	var msg outbox.Message
	if err := json.Unmarshal(payload, &msg); err != nil {
		return outbox.Message{}, fmt.Errorf("cdc: decode message: %w", err)
	}

	return msg, nil
}

// sleepFunc waits for d or until ctx is done. It is a store field so tests can
// inject a fake clock.
type sleepFunc func(ctx context.Context, d time.Duration) error

// quoteLiteral single-quotes a SQL string literal, escaping embedded quotes.
func quoteLiteral(s string) string {
	return `'` + strings.ReplaceAll(s, `'`, `''`) + `'`
}
