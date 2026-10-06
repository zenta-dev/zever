package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/outbox"
)

// claimed is one row claimed for publication.
type claimed struct {
	ID        string
	Topic     string
	Key       string
	Payload   []byte
	Headers   map[string]string
	CreatedAt time.Time
	Attempts  int
}

// Start launches the relay loop. It is idempotent and returns an error when
// no Publisher is wired.
func (d *driver) Start(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return errors.New("db: outbox closed")
	}

	if d.started {
		return nil
	}

	if d.publisher == nil {
		return fmt.Errorf("db: start: publisher is nil: %w", outbox.ErrInvalidOptions)
	}

	runCtx, cancel := context.WithCancel(ctx)

	d.cancel = cancel
	d.done = make(chan struct{})
	d.started = true

	go d.loop(runCtx, d.done)

	return nil
}

// loop drains immediately, then polls on the configured cadence until ctx is
// cancelled.
func (d *driver) loop(ctx context.Context, done chan struct{}) {
	defer close(done)

	d.pollOnce(ctx)

	ticker := time.NewTicker(d.poll)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.pollOnce(ctx)
		}
	}
}

// Close stops the relay and, when this driver owns its pool, closes it. It is
// idempotent.
func (d *driver) Close() error {
	d.mu.Lock()

	if d.closed {
		d.mu.Unlock()

		return nil
	}

	d.closed = true
	cancel := d.cancel
	done := d.done

	d.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	if done != nil {
		<-done
	}

	if !d.owns {
		return nil
	}

	return d.conn.Close(context.Background())
}

// pollOnce claims and delivers one batch, then runs retention cleanup. It
// never blocks: errors are recorded for Status.
func (d *driver) pollOnce(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}

	msgs, err := d.claim(ctx)
	if err != nil {
		d.setRelayError(err)

		return
	}

	for i := range msgs {
		if ctx.Err() != nil {
			return
		}

		d.deliver(ctx, msgs[i])
	}

	d.cleanup(ctx)

	d.emitStatusGauges(ctx)
}

// emitStatusGauges records the pending, failed, and oldest-pending-age gauges.
// It is best-effort: query failures skip emission.
func (d *driver) emitStatusGauges(ctx context.Context) {
	var st outbox.Status

	if err := d.countByStatus(ctx, &st); err != nil {
		return
	}

	oldest, ok, err := d.oldestPending(ctx)
	if err != nil {
		return
	}

	var age time.Duration
	if ok {
		age = time.Since(oldest)
	}

	d.recorder.Status(ctx, st.Pending, st.Failed, age)
}

// claim atomically claims up to BatchSize pending rows, incrementing attempts
// and taking the lock lease. Postgres uses FOR UPDATE SKIP LOCKED; sqlite
// serializes on the process-local claim mutex.
func (d *driver) claim(ctx context.Context) ([]claimed, error) {
	if err := d.checkDialect(); err != nil {
		return nil, err
	}

	if d.conn.Dialect() != "postgres" {
		d.claimMu.Lock()
		defer d.claimMu.Unlock()
	}

	now := time.Now().UTC()

	rows, err := d.conn.Query(ctx, d.claimSQL(), d.ts(now.Add(d.lock)), d.ts(now), d.batch)
	if err != nil {
		return nil, fmt.Errorf("db: claim: %w", err)
	}

	defer func() { _ = rows.Close() }()

	var out []claimed

	for rows.Next() {
		c, err := scanClaimed(rows)
		if err != nil {
			return nil, fmt.Errorf("db: claim scan: %w", err)
		}

		out = append(out, c)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("db: claim: %w", err)
	}

	return out, nil
}

// claimSQL builds the dialect-specific atomic claim statement.
func (d *driver) claimSQL() string {
	t := quoteIdent(d.table)

	skip := ""
	if d.conn.Dialect() == "postgres" {
		skip = " FOR UPDATE SKIP LOCKED"
	}

	return `UPDATE ` + t + ` SET locked_until = ?, attempts = attempts + 1 ` +
		`WHERE id IN (SELECT id FROM ` + t + ` ` +
		`WHERE status = 'pending' AND (locked_until IS NULL OR locked_until < ?) ` +
		`ORDER BY created_at LIMIT ?` + skip + `) ` +
		`RETURNING id, topic, "key", payload, headers, created_at, attempts`
}

// deliver publishes one claimed row and records the outcome.
func (d *driver) deliver(ctx context.Context, c claimed) {
	msg := outbox.Message{
		ID:        c.ID,
		Topic:     c.Topic,
		Key:       c.Key,
		Payload:   c.Payload,
		Headers:   c.Headers,
		CreatedAt: c.CreatedAt,
		Attempts:  c.Attempts,
	}

	spanCtx, finish := d.recorder.PublishSpan(ctx, c.Topic)

	err := d.publisher.Publish(spanCtx, msg)
	now := time.Now().UTC()

	switch {
	case err == nil:
		finish(outbox.OutcomeOK)
		d.recorder.Published(ctx)
		d.markProcessed(ctx, c.ID, now)
	case c.Attempts >= d.maxAttempts:
		finish(outbox.OutcomeError)
		d.markFailed(ctx, c.ID, err)
	default:
		finish(outbox.OutcomeError)
		d.markRetry(ctx, c.ID, err, now.Add(d.retry.NextDelay(c.Attempts)))
	}
}

// markProcessed records a successful publish.
func (d *driver) markProcessed(ctx context.Context, id string, now time.Time) {
	_, err := d.conn.Exec(ctx,
		`UPDATE `+quoteIdent(d.table)+` SET processed_at = ?, status = 'processed', locked_until = NULL, last_error = NULL `+
			`WHERE id = ? AND status = 'pending'`,
		d.ts(now), id,
	)
	if err != nil {
		d.setRelayError(err)
	}
}

// markFailed moves a message to the DLQ: processed_at stays NULL and status
// becomes failed, so the relay stops retrying it.
func (d *driver) markFailed(ctx context.Context, id string, cause error) {
	d.recorder.FailedTotal(ctx)

	_, err := d.conn.Exec(ctx,
		`UPDATE `+quoteIdent(d.table)+` SET status = 'failed', locked_until = NULL, last_error = ? `+
			`WHERE id = ? AND status = 'pending'`,
		cause.Error(), id,
	)
	if err != nil {
		d.setRelayError(err)
	}
}

// markRetry records a failure and delays the next attempt by the backoff.
func (d *driver) markRetry(ctx context.Context, id string, cause error, retryAt time.Time) {
	d.recorder.Retried(ctx)

	_, err := d.conn.Exec(ctx,
		`UPDATE `+quoteIdent(d.table)+` SET last_error = ?, locked_until = ? WHERE id = ? AND status = 'pending'`,
		cause.Error(), d.ts(retryAt), id,
	)
	if err != nil {
		d.setRelayError(err)
	}
}

// cleanup deletes processed rows older than Retention.
func (d *driver) cleanup(ctx context.Context) {
	cutoff := time.Now().UTC().Add(-d.retention)

	_, err := d.conn.Exec(ctx,
		`DELETE FROM `+quoteIdent(d.table)+` WHERE status = 'processed' AND processed_at IS NOT NULL AND processed_at < ?`,
		d.ts(cutoff),
	)
	if err != nil {
		d.setRelayError(err)
	}
}

// setRelayError records the most recent relay error for Status.
func (d *driver) setRelayError(err error) {
	s := err.Error()
	d.relayErr.Store(&s)
	d.recorder.RelayError(context.Background())
}

// Status reports current counters and health. It is best-effort: a query
// failure yields whatever was gathered so far.
func (d *driver) Status() outbox.Status {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultOperationTimeout)
	defer cancel()

	var st outbox.Status

	if err := d.countByStatus(ctx, &st); err != nil {
		d.setRelayError(err)
	}

	oldest, ok, err := d.oldestPending(ctx)
	if err != nil {
		d.setRelayError(err)
	}

	var age time.Duration
	if ok {
		age = time.Since(oldest)
	}

	d.recorder.Status(ctx, st.Pending, st.Failed, age)

	if ok && age > DefaultStallAfter {
		st.Stalled = true
	}

	if last, err := d.lastError(ctx); err == nil && last != "" {
		st.LastError = last
	}

	if ptr := d.relayErr.Load(); ptr != nil && *ptr != "" {
		st.LastError = *ptr
	}

	return st
}

// countByStatus fills Pending/Processed/Failed from a grouped count query.
func (d *driver) countByStatus(ctx context.Context, st *outbox.Status) error {
	rows, err := d.conn.Query(ctx,
		`SELECT status, COUNT(*) FROM `+quoteIdent(d.table)+` GROUP BY status`)
	if err != nil {
		return fmt.Errorf("db: status: %w", err)
	}

	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var status string
		var countRaw any

		if err := rows.Scan(&status, &countRaw); err != nil {
			return fmt.Errorf("db: status scan: %w", err)
		}

		n, err := coerceInt(countRaw)
		if err != nil {
			return fmt.Errorf("db: status count: %w", err)
		}

		switch status {
		case "pending":
			st.Pending = n
		case "processed":
			st.Processed = n
		case "failed":
			st.Failed = n
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("db: status: %w", err)
	}

	return nil
}

// oldestPending returns the created_at of the oldest pending row.
func (d *driver) oldestPending(ctx context.Context) (time.Time, bool, error) {
	rows, err := d.conn.Query(ctx,
		`SELECT created_at FROM `+quoteIdent(d.table)+` WHERE status = 'pending' ORDER BY created_at ASC LIMIT 1`)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("db: status: %w", err)
	}

	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		return time.Time{}, false, rows.Err()
	}

	var raw any
	if scanErr := rows.Scan(&raw); scanErr != nil {
		return time.Time{}, false, fmt.Errorf("db: status scan: %w", scanErr)
	}

	t, err := coerceTime(raw)
	if err != nil {
		return time.Time{}, false, err
	}

	return t, true, nil
}

// lastError returns the most recent stored publish error.
func (d *driver) lastError(ctx context.Context) (string, error) {
	rows, err := d.conn.Query(ctx,
		`SELECT last_error FROM `+quoteIdent(d.table)+` WHERE last_error IS NOT NULL AND last_error <> '' ORDER BY created_at DESC LIMIT 1`)
	if err != nil {
		return "", fmt.Errorf("db: status: %w", err)
	}

	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		return "", rows.Err()
	}

	var raw any
	if scanErr := rows.Scan(&raw); scanErr != nil {
		return "", fmt.Errorf("db: status scan: %w", scanErr)
	}

	return coerceString(raw), nil
}

// scanClaimed reads one RETURNING row positionally.
func scanClaimed(rows coredb.Rows) (claimed, error) {
	var (
		id, topic              string
		keyRaw, headersRaw     any
		createdRaw, attemptRaw any
		payload                []byte
	)

	if err := rows.Scan(&id, &topic, &keyRaw, &payload, &headersRaw, &createdRaw, &attemptRaw); err != nil {
		return claimed{}, err
	}

	created, err := coerceTime(createdRaw)
	if err != nil {
		return claimed{}, err
	}

	attempts, err := coerceInt(attemptRaw)
	if err != nil {
		return claimed{}, err
	}

	return claimed{
		ID:        id,
		Topic:     topic,
		Key:       coerceString(keyRaw),
		Payload:   payload,
		Headers:   decodeHeaders(coerceString(headersRaw)),
		CreatedAt: created,
		Attempts:  int(attempts),
	}, nil
}

// coerceTime converts a timestamp cell to time.Time.
func coerceTime(v any) (time.Time, error) {
	switch t := v.(type) {
	case nil:
		return time.Time{}, nil
	case time.Time:
		return t, nil
	case string:
		return time.Parse(tsLayout, t)
	case []byte:
		return time.Parse(tsLayout, string(t))
	default:
		return time.Time{}, fmt.Errorf("db: unsupported timestamp %T", v)
	}
}

// coerceInt converts an integer cell to int64.
func coerceInt(v any) (int64, error) {
	switch n := v.(type) {
	case int64:
		return n, nil
	case int32:
		return int64(n), nil
	case int:
		return int64(n), nil
	case float64:
		return int64(n), nil
	default:
		return 0, fmt.Errorf("db: unsupported integer %T", v)
	}
}

// coerceString converts a text cell to string, treating NULL as empty.
func coerceString(v any) string {
	switch s := v.(type) {
	case nil:
		return ""
	case string:
		return s
	case []byte:
		return string(s)
	default:
		return fmt.Sprintf("%v", s)
	}
}

// decodeHeaders parses a TEXT cell to headers. Empty and corrupt cells decode
// to nil: the payload is the contract, headers are metadata.
func decodeHeaders(s string) map[string]string {
	if s == "" || s == "null" {
		return nil
	}

	var m map[string]string
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil
	}

	return m
}
