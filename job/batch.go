package job

import (
	"context"
	"crypto/rand"
	"encoding/json/v2"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"
	"uuid"

	"github.com/zenta-dev/zever/log"
	"github.com/zenta-dev/zever/log/noop"
)

var (
	randReader    = rand.Reader
	triggerJitter = func() bool { return cryptoRandInt64n(10) == 0 }
)

// BatchStore persists batch progress across workers.
// Implementations must be safe for concurrent use.
type BatchStore interface {
	// Create initializes a batch record with the expected total member count.
	Create(ctx context.Context, id string, total int) error
	// IncrementCompleted records one successful member and returns updated progress.
	IncrementCompleted(ctx context.Context, id string) (completed, total int, err error)
	// IncrementFailed records one failed member.
	IncrementFailed(ctx context.Context, id string) error
	// IncrementFailedBy records n failed members, used for lost members at settlement.
	IncrementFailedBy(ctx context.Context, id string, n int) error
	// Get returns completed, failed, and total counts for a batch.
	Get(ctx context.Context, id string) (completed, failed, total int, err error)
}

// BatchID uniquely identifies a batch.
// Its zero value is not valid; use NewBatch to generate one.
type BatchID uuid.UUID

func newBatchID() BatchID {
	return BatchID(uuid.NewV7())
}

// String returns the canonical UUID string form of BatchID.
func (b BatchID) String() string {
	return uuid.UUID(b).String()
}

type batchedJob struct {
	name string
	args any
}

type batchCallback struct {
	onComplete   func(ctx context.Context)
	onFailure    func(ctx context.Context, jobName string, err error)
	store        BatchStore
	deadline     time.Time
	settling     atomic.Bool
	completeOnce *sync.Once
	logger       log.Logger
}

// batchCallbacks is process-global rather than scoped to a *Dispatcher
// instance: reportBatchFailure/reportBatchSuccess run from the job
// execution path with only a batch ID and store, no Dispatcher reference,
// so instance-scoping would require threading *Dispatcher through the
// entire job-completion call chain. This is safe in practice because keys
// are BatchID.String() -- a UUIDv7, not an attacker-controlled or
// low-entropy value -- so two unrelated batches (even across separate
// Dispatcher instances in the same process) colliding on the same key is a
// UUID-collision event, not a realistic risk from sharing the map.
var batchCallbacks = struct {
	sync.Mutex
	m map[string]*batchCallback
}{m: make(map[string]*batchCallback)}

var sweepBatchesSettled atomic.Int64

// SweepBatchCallbacksCalls returns the number of batch settlement sweeps performed.
// It is intended for observability and tests.
func SweepBatchCallbacksCalls() int64 { return sweepBatchesSettled.Load() }

// Batch groups jobs dispatched together with shared completion callbacks.
// Dispatcher, store, jobs, and callbacks are unexported; only ID is inspected directly.
type Batch struct {
	// ID uniquely identifies the batch across dispatcher, store, and queue headers.
	ID         BatchID
	dispatcher *Dispatcher
	store      BatchStore
	jobs       []batchedJob
	onComplete func(ctx context.Context)
	onFailure  func(ctx context.Context, jobName string, err error)
	deadline   time.Time
}

const defaultSettleTimeout = 5 * time.Minute

// NewBatch creates a batch bound to dispatcher d and store.
// Settle timeout defaults to 5m when dispatcher timeout is non-positive.
func NewBatch(d *Dispatcher, store BatchStore) *Batch {
	timeout := d.SettleTimeout
	if timeout <= 0 {
		timeout = defaultSettleTimeout
	}

	return &Batch{
		ID:         newBatchID(),
		dispatcher: d,
		store:      store,
		deadline:   time.Now().Add(timeout),
	}
}

// Add queues jobName with args as a batch member.
// It returns the batch for chaining.
func (b *Batch) Add(jobName string, args any) *Batch {
	b.jobs = append(b.jobs, batchedJob{jobName, args})

	return b
}

// Then registers fn invoked once when all batch members succeed.
// It returns the batch for chaining.
func (b *Batch) Then(fn func(ctx context.Context)) *Batch {
	b.onComplete = fn

	return b
}

// Catch registers fn invoked for each failed or lost batch member.
// It returns the batch for chaining.
func (b *Batch) Catch(fn func(ctx context.Context, jobName string, err error)) *Batch {
	b.onFailure = fn

	return b
}

// Dispatch marshals members, records the batch, and pushes jobs to the queue.
// It reports dispatch failures to the store and failure callback, returning joined errors.
func (b *Batch) Dispatch(ctx context.Context) error {
	type queuedJob struct {
		payload []byte
		topic   string
		name    string
	}

	queued := make([]queuedJob, 0, len(b.jobs))

	for _, j := range b.jobs {
		payload, err := json.Marshal(j.args)
		if err != nil {
			return fmt.Errorf("job: %q marshaling batch args: %w", j.name, err)
		}

		def, ok := Lookup(j.name)
		if !ok {
			return &UnknownJobError{Name: j.name}
		}

		queued = append(queued, queuedJob{payload, def.priority.String(), j.name})
	}

	if err := b.store.Create(ctx, b.ID.String(), len(b.jobs)); err != nil {
		return fmt.Errorf("job: create batch %q: %w", b.ID.String(), err)
	}

	batchCallbacks.Lock()
	batchCallbacks.m[b.ID.String()] = &batchCallback{
		onComplete:   b.onComplete,
		onFailure:    b.onFailure,
		store:        b.store,
		deadline:     b.deadline,
		completeOnce: new(sync.Once),
		logger:       b.dispatcher.log(),
	}
	batchCallbacks.Unlock()

	var failures []error

	for _, q := range queued {
		headers := map[string]string{headerJobName: q.name, "batch_id": b.ID.String()}

		if err := b.dispatcher.Q.Push(ctx, q.topic, q.payload, headers); err != nil {
			failures = append(failures, fmt.Errorf("job: %q dispatching batch member: %w", q.name, err))

			if incErr := b.store.IncrementFailed(ctx, b.ID.String()); incErr != nil {
				b.dispatcher.log().Warn().Str("batch", b.ID.String()).Str("job", q.name).Err(incErr).Msg("batch: increment failed")
			} else {
				batchCallbacks.Lock()
				cb := batchCallbacks.m[b.ID.String()]
				batchCallbacks.Unlock()

				if cb != nil && cb.onFailure != nil {
					cb.onFailure(ctx, q.name, err)
				}
			}
		}
	}

	if len(failures) > 0 {
		if _, settled := batchSettle(ctx, b.store, b.ID.String()); settled {
			batchCallbacks.Lock()
			delete(batchCallbacks.m, b.ID.String())
			batchCallbacks.Unlock()
		}

		return errors.Join(failures...)
	}

	if len(queued) == 0 {
		batchCallbacks.Lock()
		cb := batchCallbacks.m[b.ID.String()]
		batchCallbacks.Unlock()

		failed, settled := batchSettle(ctx, b.store, b.ID.String())

		if settled && failed == 0 && cb != nil && cb.onComplete != nil {
			cb.completeOnce.Do(func() { cb.onComplete(ctx) })
		}

		batchCallbacks.Lock()
		delete(batchCallbacks.m, b.ID.String())
		batchCallbacks.Unlock()
	}

	return nil
}

func reportBatchResult(ctx context.Context, store BatchStore, batchID, jobName string, jobErr error) {
	if jobErr != nil {
		reportBatchFailure(ctx, store, batchID, jobName, jobErr)

		return
	}

	reportBatchSuccess(ctx, store, batchID)
}

func reportBatchFailure(ctx context.Context, store BatchStore, batchID, jobName string, jobErr error) {
	if err := store.IncrementFailed(ctx, batchID); err != nil {
		batchLogger(batchID).Warn().Str("batch", batchID).Str("job", jobName).Err(err).Msg("batch: increment failed")

		return
	}

	batchCallbacks.Lock()
	cb, ok := batchCallbacks.m[batchID]
	settling := ok && cb != nil && cb.settling.Load()
	batchCallbacks.Unlock()

	if settling {
		batchLogger(batchID).
			Warn().
			Str("batch", batchID).
			Msg("batch: settling in progress; deferring failure settlement")

		return
	}

	if ok && cb != nil && cb.onFailure != nil {
		cb.onFailure(ctx, jobName, jobErr)
	}

	if _, settled := batchSettle(ctx, store, batchID); settled {
		batchCallbacks.Lock()

		if cur, ok := batchCallbacks.m[batchID]; ok && !cur.settling.Load() {
			delete(batchCallbacks.m, batchID)
		}

		batchCallbacks.Unlock()
	}
}

func reportBatchSuccess(ctx context.Context, store BatchStore, batchID string) {
	if _, _, err := store.IncrementCompleted(ctx, batchID); err != nil {
		batchLogger(batchID).
			Warn().
			Str("batch", batchID).
			Msg("batch: increment completed error")

		return
	}

	batchCallbacks.Lock()
	settling := false

	if cb, ok := batchCallbacks.m[batchID]; ok && cb != nil && cb.settling.Load() {
		settling = true
	}

	batchCallbacks.Unlock()

	if settling {
		batchLogger(batchID).
			Warn().
			Str("batch", batchID).
			Msg("batch: settling in progress; deferring final check")

		return
	}

	failed, settled := batchSettle(ctx, store, batchID)
	if !settled {
		return
	}

	batchCallbacks.Lock()
	cb, ok := batchCallbacks.m[batchID]

	if ok && cb != nil && !cb.settling.Load() {
		delete(batchCallbacks.m, batchID)
	}

	settling = ok && cb != nil && cb.settling.Load()
	batchCallbacks.Unlock()

	if ok && !settling && cb != nil && failed == 0 && cb.onComplete != nil {
		cb.completeOnce.Do(func() { cb.onComplete(ctx) })
	}
}

func batchLogger(batchID string) log.Logger {
	batchCallbacks.Lock()
	cb := batchCallbacks.m[batchID]
	batchCallbacks.Unlock()

	if cb != nil {
		return cb.logger
	}

	return noop.New()
}

func batchSettle(ctx context.Context, store BatchStore, id string) (failed int, settled bool) {
	completed, failed, total, err := store.Get(ctx, id)
	if err != nil {
		return 0, false
	}

	return failed, completed+failed >= total
}

func cryptoRandInt64n(n int64) int64 {
	if n <= 0 {
		return 0
	}

	v, err := rand.Int(randReader, big.NewInt(n))
	if err != nil {
		return 0
	}

	return v.Int64()
}

func sleepWithContext(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		select {
		case <-ctx.Done():
			return false
		default:
			return true
		}
	}

	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func jitterDuration(d time.Duration) time.Duration {
	return time.Duration(cryptoRandInt64n(int64(d)))
}

func settleDueBatches(ctx context.Context) {
	sweepBatchesSettled.Add(1)

	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc

		ctx, cancel = context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
	}

	// Jitter up to 50ms to avoid thundering herd when multiple workers sweep
	// at the same tick.
	if triggerJitter() {
		if !sleepWithContext(ctx, jitterDuration(50*time.Millisecond)) {
			return
		}
	}

	type due struct {
		id string
		cb *batchCallback
	}

	var expired []due

	batchCallbacks.Lock()
	now := time.Now()

	for id, cb := range batchCallbacks.m {
		if cb != nil && !cb.deadline.IsZero() && now.After(cb.deadline) {
			expired = append(expired, due{id, cb})
		}
	}

	batchCallbacks.Unlock()

	for _, d := range expired {
		settleExpired(ctx, d.id, d.cb)
	}
}

func settleExpired(ctx context.Context, id string, cb *batchCallback) {
	if cb == nil || cb.store == nil {
		return
	}

	if !acquireSettling(id, cb) {
		return
	}

	defer cb.settling.Store(false)

	if !loadAndFailLost(ctx, id, cb) {
		return
	}

	finalizeExpired(ctx, id, cb)
}

func acquireSettling(id string, cb *batchCallback) bool {
	batchCallbacks.Lock()
	cur, ok := batchCallbacks.m[id]

	if !ok || cur != cb {
		batchCallbacks.Unlock()

		return false
	}

	batchCallbacks.Unlock()

	return cb.settling.CompareAndSwap(false, true)
}

func loadAndFailLost(ctx context.Context, id string, cb *batchCallback) bool {
	completed, failed, total, err := cb.store.Get(ctx, id)
	if err != nil {
		cb.logger.Warn().Str("batch", id).Err(err).Msg("batch: settlement check error")

		return false
	}

	lost := total - completed - failed
	if lost <= 0 {
		return true
	}

	if err := cb.store.IncrementFailedBy(ctx, id, lost); err != nil {
		cb.logger.Warn().Str("batch", id).Err(err).Msg("batch: increment failed (settlement timeout)")

		return true
	}

	if cb.onFailure != nil {
		for i := 0; i < lost; i++ {
			cb.onFailure(ctx, "", ErrMemberLost)
		}
	}

	return true
}

func finalizeExpired(ctx context.Context, id string, cb *batchCallback) {
	completed, failed, total, err := cb.store.Get(ctx, id)
	if err != nil {
		cb.
			logger.
			Warn().
			Str("batch", id).
			Err(err).
			Msg("batch: settlement recheck error")

		return
	}

	if completed+failed < total {
		cb.
			logger.
			Warn().
			Str("batch", id).
			Int("remaining", total-completed-failed).
			Msg("batch: not settlement; unaccounted members remain")

		return
	}

	if failed == 0 && cb.onComplete != nil {
		cb.completeOnce.Do(func() { cb.onComplete(ctx) })
	}

	batchCallbacks.Lock()

	if cur, ok := batchCallbacks.m[id]; ok && cur == cb {
		delete(batchCallbacks.m, id)
	}

	batchCallbacks.Unlock()

	cb.
		logger.
		Info().
		Str("batch", id).
		Int("failed", total-completed-failed).
		Msg("batch: settled by timeout")
}

// SweepBatchCallbacks settles batches whose deadlines have passed.
// Lost members are marked failed so completion or failure callbacks fire exactly once.
func SweepBatchCallbacks(ctx context.Context) {
	settleDueBatches(ctx)
}
