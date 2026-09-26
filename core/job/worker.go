package job

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/zenta-dev/zever/internal/retry"
	"github.com/zenta-dev/zever/log"
	"github.com/zenta-dev/zever/log/noop"
	"github.com/zenta-dev/zever/queue"
)

// DeadLetterFunc handles jobs that exhaust retries or are unregistered.
// It receives job name, payload, terminal error, and attempt count.
type DeadLetterFunc func(ctx context.Context, jobName string, payload []byte, err error, attempts int)

// Worker polls configured queues and executes registered jobs concurrently.
// Use Run to start polling until context cancellation.
type Worker struct {
	// Q is the queue backend used for polling, acking, and requeuing jobs.
	Q queue.Queue
	// Queues lists topics polled in order on each iteration.
	Queues []string
	// Concurrency limits in-flight job handlers. Defaults to 10 when non-positive.
	Concurrency int
	// OnDeadLetter is invoked for jobs that exhaust retries or are unregistered.
	OnDeadLetter DeadLetterFunc
	// BatchStore reports batch member outcomes when set.
	BatchStore BatchStore
	// Logger emits job lifecycle events. Defaults to a no-op logger when nil.
	Logger log.Logger
	// DrainTimeout bounds handler execution and graceful shutdown. Defaults to 30s when non-positive.
	DrainTimeout time.Duration
}

const defaultDrainTimeout = 30 * time.Second

// DefaultSweepInterval is the batch-settlement sweep cadence for settling due batches.
const DefaultSweepInterval = time.Second

// DefaultSweepTimeout bounds one detached batch-settlement sweep.
const DefaultSweepTimeout = 5 * time.Second

// DefaultMaxPollWait caps the empty-poll backoff.
const DefaultMaxPollWait = 5 * time.Second

// DefaultPollBaseDelay is the initial empty-poll backoff.
const DefaultPollBaseDelay = time.Millisecond

func (w *Worker) drainTimeout() time.Duration {
	if w.DrainTimeout > 0 {
		return w.DrainTimeout
	}

	return defaultDrainTimeout
}

func (w *Worker) waitDrain(inflight *sync.WaitGroup) {
	done := make(chan struct{})

	go func() {
		inflight.Wait()
		close(done)
	}()

	timer := time.NewTimer(w.drainTimeout())
	defer timer.Stop()

	select {
	case <-done:
	case <-timer.C:
	}
}

func effectiveAttempt(msg queue.Message) int {
	if v, err := strconv.Atoi(msg.Headers[headerAttempt]); err == nil {
		return v
	}

	return msg.Attempt
}

// Run polls queues and executes handlers until context cancellation.
// It starts a sweep loop for batch settlement and a run loop bounded by a semaphore.
func (w *Worker) Run(ctx context.Context) error {
	concurrency := w.Concurrency
	if concurrency <= 0 {
		concurrency = 10
	}

	sem := make(chan struct{}, concurrency)

	var inflight sync.WaitGroup

	sweep := time.NewTicker(DefaultSweepInterval)
	defer sweep.Stop()

	go w.sweepLoop(ctx, sweep)

	return w.runLoop(ctx, sem, &inflight)
}

func (w *Worker) sweepLoop(ctx context.Context, sweep *time.Ticker) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-sweep.C:
			w.sweepBatches(ctx)
		}
	}
}

func (w *Worker) runLoop(ctx context.Context, sem chan struct{}, inflight *sync.WaitGroup) error {
	pollAttempt := 0

	maxPollWait := DefaultMaxPollWait

	for {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			w.waitDrain(inflight)

			return nil
		}

		msg, topic, err := w.popNextAvailable(ctx)
		if errors.Is(err, queue.ErrEmpty) {
			<-sem

			if w.handleEmpty(ctx, inflight, &pollAttempt, maxPollWait) {
				return nil
			}

			continue
		}

		if err != nil {
			<-sem
			w.waitDrain(inflight)

			if errors.Is(err, context.Canceled) || ctx.Err() != nil {
				err = nil
			}

			return err
		}

		pollAttempt = 0

		w.launchHandler(ctx, msg, topic, sem, inflight)
	}
}

func (w *Worker) sweepBatches(ctx context.Context) {
	detached := context.WithoutCancel(ctx)

	sweepCtx, cancel := context.WithTimeout(detached, DefaultSweepTimeout) //nolint:contextcheck // detached timeout for sweep
	defer cancel()

	settleDueBatches(sweepCtx)
}

// handleEmpty waits before the next poll after an empty queue, using an
// increasing attempt counter (consecutive empty polls) rather than a
// mutated duration: the first call in a run of empty polls waits 0 (poll
// immediately), and every subsequent call waits nextPollWait(*attempt,
// maxPollWait), matching the doubling sequence the previous
// mutated-duration implementation produced (0, 1ms, 2ms, 4ms, ..., capped
// at maxPollWait).
func (w *Worker) handleEmpty(ctx context.Context, inflight *sync.WaitGroup, attempt *int, maxPollWait time.Duration) bool {
	wait := time.Duration(0)
	if *attempt > 0 {
		wait = nextPollWait(*attempt, maxPollWait)
	}

	select {
	case <-time.After(wait):
	case <-ctx.Done():
		w.waitDrain(inflight)

		return true
	}

	*attempt++

	return false
}

// nextPollWait returns the poll-wait delay for the given (1-based) attempt:
// BaseDelay doubled per attempt, capped at maxPollWait.
func nextPollWait(attempt int, maxPollWait time.Duration) time.Duration {
	return retry.Policy{BaseDelay: DefaultPollBaseDelay, Multiplier: 2, MaxDelay: maxPollWait}.NextDelay(attempt)
}

func (w *Worker) launchHandler(ctx context.Context, msg queue.Message, topic string, sem chan struct{}, inflight *sync.WaitGroup) {
	inflight.Add(1)

	go func(msg queue.Message, topic string) {
		defer func() {
			<-sem
			inflight.Done()
		}()

		detached := context.WithoutCancel(ctx)

		handlerCtx, cancel := context.WithTimeout(detached, w.drainTimeout())
		defer cancel()

		w.process(handlerCtx, msg, topic)
	}(msg, topic)
}

func (w *Worker) popNextAvailable(ctx context.Context) (queue.Message, string, error) {
	if len(w.Queues) == 0 {
		return queue.Message{}, "", queue.ErrEmpty
	}

	for _, topic := range w.Queues {
		select {
		case <-ctx.Done():
			return queue.Message{}, "", ctx.Err()
		default:
		}

		msg, err := w.Q.Pop(ctx, topic)
		if !errors.Is(err, queue.ErrEmpty) {
			return msg, topic, err
		}
	}

	return queue.Message{}, "", queue.ErrEmpty
}

func (w *Worker) log() log.Logger {
	if w.Logger != nil {
		return w.Logger
	}

	return noop.New()
}

func (w *Worker) process(ctx context.Context, msg queue.Message, topic string) {
	if msg.Headers == nil {
		msg.Headers = make(map[string]string)
	}

	jobName := msg.Headers[headerJobName]
	def, ok := Lookup(jobName)

	if !ok {
		batchID := msg.Headers["batch_id"]
		isBatch := batchID != "" && w.BatchStore != nil
		attempt := effectiveAttempt(msg)

		w.handleDeadLetter(context.WithoutCancel(ctx), msg, batchID, jobName, &UnknownJobError{Name: jobName}, attempt, isBatch)

		return
	}

	h := w.wrapHandler(def.handler)

	start := time.Now()
	attempt := effectiveAttempt(msg)
	err := w.invokeHandler(ctx, h, msg.Payload, jobName)

	w.log().Info().Str("job", jobName).Dur("duration", time.Since(start)).Int("attempt", attempt).Err(err).Msg("job: finished")

	w.handleResult(ctx, msg, topic, jobName, def, attempt, err)
}

func (w *Worker) wrapHandler(h Handler) Handler {
	mu.RLock()
	defer mu.RUnlock()

	for _, m := range slices.Backward(middleware) {
		h = m(h)
	}

	return h
}

func (w *Worker) invokeHandler(ctx context.Context, h Handler, payload []byte, jobName string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			err = fmt.Errorf("%w: %q handler panic: %v\n%s", ErrHandlerPanic, jobName, r, stack)

			w.log().Warn().Str("job", jobName).Any("panic", r).Msg("job: recovering from handler panic")
		}
	}()

	return h(ctx, payload)
}

func (w *Worker) handleResult(
	ctx context.Context,
	msg queue.Message,
	topic, jobName string,
	def Definition,
	attempt int,
	handlerErr error,
) {
	opCtx := context.WithoutCancel(ctx)
	batchID := msg.Headers["batch_id"]
	isBatch := batchID != "" && w.BatchStore != nil

	if handlerErr == nil {
		w.handleSuccess(opCtx, msg, batchID, jobName, isBatch)

		return
	}

	policy := def.policy

	if attempt >= policy.MaxAttempts {
		w.handleDeadLetter(opCtx, msg, batchID, jobName, handlerErr, attempt, isBatch)

		return
	}

	w.handleRetry(opCtx, msg, topic, jobName, attempt, policy)
}

func (w *Worker) handleSuccess(ctx context.Context, msg queue.Message, batchID, jobName string, isBatch bool) {
	if err := w.Q.Ack(ctx, msg); err != nil {
		w.log().Warn().Str("job", jobName).Err(err).Msg("job: ack error; leaving in flight for redelivery")

		return
	}

	if isBatch {
		reportBatchResult(ctx, w.BatchStore, batchID, jobName, nil)
	}
}

func (w *Worker) handleDeadLetter(
	ctx context.Context,
	msg queue.Message,
	batchID, jobName string,
	handlerErr error,
	attempt int,
	isBatch bool,
) {
	if err := w.Q.Ack(ctx, msg); err != nil {
		w.log().Warn().Str("job", jobName).Err(err).Msg("job: deadletter ack error; leaving in flight for redelivery")

		return
	}

	if isBatch {
		reportBatchResult(ctx, w.BatchStore, batchID, jobName, handlerErr)
	}

	if w.OnDeadLetter != nil {
		w.OnDeadLetter(ctx, jobName, msg.Payload, handlerErr, attempt)
	}
}

func (w *Worker) handleRetry(ctx context.Context, msg queue.Message, topic, jobName string, attempt int, policy RetryPolicy) {
	headers := msg.Headers.Clone()
	headers[headerAttempt] = strconv.Itoa(attempt + 1)

	if err := w.Q.PushDelayed(ctx, topic, msg.Payload, headers, policy.Backoff(attempt)); err != nil {
		w.log().Warn().Str("job", jobName).Err(err).Msg("job: requeue after failure error; leaving in flight for redelivery")

		return
	}

	if err := w.Q.Ack(ctx, msg); err != nil {
		w.log().Warn().Str("job", jobName).Err(err).Msg("job: ack-after-requeue error; duplicate may be delivered after visibility timeout")
	}
}
