// Package jobs implements the background jobs declared in schema/todo.zen.
package jobs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/log"
)

// SendOverdueDigestArgs is the payload of the SendOverdueDigest job declared
// in the schema. The job takes no parameters, so the payload is empty.
type SendOverdueDigestArgs struct{}

// OverdueThreshold is how old an undone note must be to count as overdue.
const OverdueThreshold = 24 * time.Hour

// CountOverdue returns the number of notes with done=false and created_at
// older than now minus OverdueThreshold.
func CountOverdue(ctx context.Context, database db.DB, now time.Time) (int, error) {
	cutoff := now.UTC().Add(-OverdueThreshold).Format(time.RFC3339Nano)

	rows, err := database.Query(ctx,
		`SELECT COUNT(*) FROM notes WHERE done = 0 AND created_at < ?`,
		cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("[jobs] count overdue: %w", err)
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return 0, fmt.Errorf("[jobs] count overdue: %w", err)
		}
		return 0, errors.New("[jobs] count overdue: no rows")
	}
	var n int64
	if err := rows.Scan(&n); err != nil {
		return 0, fmt.Errorf("[jobs] count overdue: %w", err)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("[jobs] count overdue: %w", err)
	}
	return int(n), nil
}

// RunDigest counts overdue notes and logs the count. It returns the count so
// tests can assert on it without reading logs.
func RunDigest(ctx context.Context, database db.DB, logger log.Logger) (int, error) {
	n, err := CountOverdue(ctx, database, time.Now().UTC())
	if err != nil {
		return 0, err
	}
	logger.Info().Int("overdue_notes", n).Msg("overdue digest")
	return n, nil
}

// Handler returns the job.Register-compatible handler for SendOverdueDigest.
// It closes over the resolved database and logger so the global job registry
// stays free of hidden state.
func Handler(database db.DB, logger log.Logger) func(ctx context.Context, args SendOverdueDigestArgs) error {
	return func(ctx context.Context, _ SendOverdueDigestArgs) error {
		_, err := RunDigest(ctx, database, logger)
		return err
	}
}
