package job

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"
	"fmt"
	"time"

	"github.com/zenta-dev/zever/log"
	"github.com/zenta-dev/zever/log/noop"
	"github.com/zenta-dev/zever/queue"
)

// Dispatcher enqueues registered jobs onto a queue with optional delay and uniqueness.
// Use Q as the destination queue and Logger for lock-release warnings.
type Dispatcher struct {
	// Q receives marshaled job payloads and headers.
	Q queue.Queue

	// UniqueLocker deduplicates dispatches keyed by UniqueBy.
	UniqueLocker *UniqueLocker
	// UniqueTTL bounds each uniqueness lock and defaults to 24h when non-positive.
	UniqueTTL time.Duration
	// SettleTimeout allows queued side effects to settle before dispatch returns.
	SettleTimeout time.Duration

	// Logger reports unique lock release errors and defaults to noop.
	Logger log.Logger
}

func (d *Dispatcher) log() log.Logger {
	if d.Logger != nil {
		return d.Logger
	}
	return noop.New()
}

type dispatchOptions struct {
	delay    time.Duration
	at       *time.Time
	uniqueBy string
}

// DispatchOption customizes a single Dispatch call with delay or uniqueness.
type DispatchOption func(*dispatchOptions)

// In delays dispatch by d before the job becomes visible.
// Non-positive delays dispatch immediately.
func In(d time.Duration) DispatchOption {
	return func(do *dispatchOptions) { do.delay = d }
}

// At schedules dispatch for time t.
// Past times dispatch immediately.
func At(t time.Time) DispatchOption {
	return func(do *dispatchOptions) { do.at = &t }
}

// UniqueBy deduplicates dispatches sharing jobName and key.
// A duplicate dispatch returns nil without enqueueing while the lock is held.
func UniqueBy(key string) DispatchOption {
	return func(do *dispatchOptions) { do.uniqueBy = key }
}

// Dispatch looks up jobName, marshals args, acquires the unique lock and skips if locked.
// It calls PushDelayed for a positive delay or At time, else Push, and releases the lock on push error.
func (d *Dispatcher) Dispatch(
	ctx context.Context,
	jobName string,
	args any,
	opts ...DispatchOption,
) error {
	def, ok := Lookup(jobName)
	if !ok {
		return &UnknownJobError{Name: jobName}
	}

	var o dispatchOptions
	for _, fn := range opts {
		fn(&o)
	}

	payload, err := json.Marshal(args)
	if err != nil {
		return fmt.Errorf("job: %q marshal: %w", jobName, err)
	}

	headers := map[string]string{headerJobName: jobName}

	acquired := false

	var uid string

	if o.uniqueBy != "" {
		if d.UniqueLocker == nil {
			return fmt.Errorf("%w for job %q", ErrUniqueLockerNil, jobName)
		}

		uid = uniqueID(jobName, o.uniqueBy)

		ttl := d.uniqueTTL()

		locked, lockErr := d.UniqueLocker.Acquire(ctx, uid, ttl)
		if lockErr != nil {
			return fmt.Errorf("job: %q unique lock: %w", jobName, lockErr)
		}

		if !locked {
			return nil
		}

		acquired = true

		headers[headerUniqueID] = uid
	}

	delay := o.delay
	if o.at != nil {
		delay = time.Until(*o.at)
	}

	if delay > 0 {
		err = d.Q.PushDelayed(ctx, def.priority.String(), payload, headers, delay)
	} else {
		err = d.Q.Push(ctx, def.priority.String(), payload, headers)
	}

	if err != nil && acquired {
		if rerr := d.UniqueLocker.Release(ctx, uid); rerr != nil {
			d.log().Warn().Str("job", jobName).Err(rerr).Msg("job: unique lock release error")
		}
	}

	return err
}

func (d *Dispatcher) uniqueTTL() time.Duration {
	if d.UniqueTTL > 0 {
		return d.UniqueTTL
	}

	return 24 * time.Hour
}

func uniqueID(jobName, key string) string {
	h := sha256.New()
	_, _ = h.Write([]byte(jobName))
	_, _ = h.Write([]byte(":"))
	_, _ = h.Write([]byte(key))

	return hex.EncodeToString(h.Sum(nil))
}
