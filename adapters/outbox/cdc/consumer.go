package cdc

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pglogrepl"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgproto3"

	"github.com/zenta-dev/zever/core/outbox"
)

// publishFunc delivers one replicated message. It matches
// outbox.Publisher.Publish.
type publishFunc func(ctx context.Context, msg outbox.Message) error

// loop receives replication messages until ctx is cancelled or the stream
// breaks. It handles keepalives and XLogData, publishing matching logical
// messages and acknowledging the LSN only after a successful publish.
func (s *store) loop(ctx context.Context, conn *pgconn.PgConn, done chan struct{}) {
	defer close(done)

	for {
		if ctx.Err() != nil {
			return
		}

		rawMsg, err := conn.ReceiveMessage(ctx)
		if err != nil {
			if ctx.Err() == nil {
				s.setRelayError(ctx, err)
			}

			return
		}

		if errMsg, ok := rawMsg.(*pgproto3.ErrorResponse); ok {
			s.setRelayError(ctx, pgconn.ErrorResponseToPgError(errMsg))

			return
		}

		msg, ok := rawMsg.(*pgproto3.CopyData)
		if !ok || len(msg.Data) == 0 {
			continue
		}

		switch msg.Data[0] {
		case pglogrepl.PrimaryKeepaliveMessageByteID:
			s.handleKeepalive(ctx, conn, msg.Data[1:])
		case pglogrepl.XLogDataByteID:
			s.handleXLogData(ctx, conn, msg.Data[1:])
		}
	}
}

// handleKeepalive replies to a keepalive that requests a status update. It
// acknowledges only the position already covered by published messages, so a
// keepalive never advances past an unacknowledged message.
func (s *store) handleKeepalive(ctx context.Context, conn *pgconn.PgConn, data []byte) {
	pkm, err := pglogrepl.ParsePrimaryKeepaliveMessage(data)
	if err != nil {
		s.setRelayError(ctx, err)

		return
	}

	if !pkm.ReplyRequested {
		return
	}

	if err := pglogrepl.SendStandbyStatusUpdate(ctx, conn, pglogrepl.StandbyStatusUpdate{
		WALWritePosition: pglogrepl.LSN(s.lastLSN.Load()),
		ReplyRequested:   true,
	}); err != nil {
		s.setRelayError(ctx, err)
	}
}

// handleXLogData parses one WAL record and, when it carries a logical message
// with a matching prefix, publishes it and acknowledges the new LSN.
func (s *store) handleXLogData(ctx context.Context, conn *pgconn.PgConn, data []byte) {
	xld, err := pglogrepl.ParseXLogData(data)
	if err != nil {
		s.setRelayError(ctx, err)

		return
	}

	logicalMsg, err := pglogrepl.Parse(xld.WALData)
	if err != nil {
		s.setRelayError(ctx, err)

		return
	}

	ldm, ok := logicalMsg.(*pglogrepl.LogicalDecodingMessage)
	if !ok {
		return
	}

	publisher := s.relayPublisher()
	if publisher == nil {
		// Start refuses to run without a publisher and SetPublisher ignores
		// nil, so this is unreachable; leave the LSN unacknowledged rather than
		// dropping the message if that contract ever changes.
		s.setRelayError(ctx, errors.New("cdc: consume: no publisher attached"))

		return
	}

	advanced, err := s.handleMessage(ctx, ldm.Prefix, ldm.Content, publisher.Publish)
	if err != nil {
		s.setRelayError(ctx, err)

		return
	}

	if !advanced {
		return
	}

	if ldm.LSN > pglogrepl.LSN(s.lastLSN.Load()) {
		s.lastLSN.Store(uint64(ldm.LSN))
	}

	if err := pglogrepl.SendStandbyStatusUpdate(ctx, conn, pglogrepl.StandbyStatusUpdate{
		WALWritePosition: pglogrepl.LSN(s.lastLSN.Load()),
	}); err != nil {
		s.setRelayError(ctx, err)
	}
}

// handleMessage processes one decoded logical message. It returns true when
// the replication LSN may advance past the message: either the prefix did not
// match (a foreign message, skipped without publishing) or the message was
// published successfully. It returns false when publishing exhausted the
// attempt budget, leaving the message unacknowledged so a later run
// redelivers it. Publish failures are retried with shared/retry backoff and
// surfaced in Status.
func (s *store) handleMessage(ctx context.Context, prefix string, payload []byte, publish publishFunc) (bool, error) {
	if prefix != s.prefix {
		return true, nil
	}

	msg, err := decodeMessage(payload)
	if err != nil {
		return false, err
	}

	spanCtx, finish := s.recorder.ConsumeSpan(ctx, msg.Topic, msg.ID)
	defer finish()

	for attempt := 1; ; attempt++ {
		pubErr := publish(spanCtx, msg)
		if pubErr == nil {
			s.recorder.Published(ctx)
			s.processed.Add(1)

			return true, nil
		}

		s.setRelayError(ctx, pubErr)

		if attempt >= s.maxAttempts {
			s.recorder.FailedTotal(ctx)
			s.failed.Add(1)

			return false, nil
		}

		s.recorder.Retried(ctx)

		if err := s.sleep(ctx, s.retry.NextDelay(attempt)); err != nil {
			return false, err
		}
	}
}

// sleepCtx waits for d or until ctx is done, whichever comes first.
func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}

	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
