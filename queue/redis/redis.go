package redis

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	goredis "github.com/redis/go-redis/v9"

	zredis "github.com/zenta-dev/zever/internal/redis"
	"github.com/zenta-dev/zever/internal/retry"
	"github.com/zenta-dev/zever/queue"
)

var (
	jsonMarshal   = json.Marshal
	jsonUnmarshal = json.Unmarshal
)

// DefaultPingTimeout bounds the startup connectivity check.
const DefaultPingTimeout = 3 * time.Second

// DefaultVisibilityTimeout is the default claim lease when unset.
const DefaultVisibilityTimeout = 30 * time.Second

// DefaultPollTimeout is the default Pop wait when unset.
const DefaultPollTimeout = 5 * time.Second

// DefaultBlockTimeout is the BLPop block slice capped by the poll timeout.
const DefaultBlockTimeout = 100 * time.Millisecond

// DefaultBufferBaseDelay is the initial wait-for-buffer backoff.
const DefaultBufferBaseDelay = 10 * time.Millisecond

// DefaultBufferMaxDelay caps the wait-for-buffer backoff.
const DefaultBufferMaxDelay = 200 * time.Millisecond

// bufferBackoffPolicy computes the poll delay while waitForBuffer waits for
// buffer capacity to free up: exponential backoff from 10ms, doubling each
// attempt, capped at 200ms, with up to 10% one-sided additive jitter.
var bufferBackoffPolicy = retry.Policy{
	BaseDelay:  DefaultBufferBaseDelay,
	Multiplier: 2,
	MaxDelay:   DefaultBufferMaxDelay,
	Jitter:     0.1,
	JitterMode: retry.JitterAdditive,
}

type redisClient interface {
	RPush(ctx context.Context, key string, values ...any) *goredis.IntCmd
	ZAdd(ctx context.Context, key string, members ...goredis.Z) *goredis.IntCmd
	LLen(ctx context.Context, key string) *goredis.IntCmd
	ZCard(ctx context.Context, key string) *goredis.IntCmd
	BLPop(ctx context.Context, timeout time.Duration, keys ...string) *goredis.StringSliceCmd
	HSet(ctx context.Context, key string, values ...any) *goredis.IntCmd
	HDel(ctx context.Context, key string, fields ...string) *goredis.IntCmd
	HLen(ctx context.Context, key string) *goredis.IntCmd
	Pipeline() goredis.Pipeliner
	Eval(ctx context.Context, script string, keys []string, args ...any) *goredis.Cmd
	EvalSha(ctx context.Context, sha1 string, keys []string, args ...any) *goredis.Cmd
	EvalRO(ctx context.Context, script string, keys []string, args ...any) *goredis.Cmd
	EvalShaRO(ctx context.Context, sha1 string, keys []string, args ...any) *goredis.Cmd
	ScriptExists(ctx context.Context, hashes ...string) *goredis.BoolSliceCmd
	ScriptLoad(ctx context.Context, script string) *goredis.StringCmd
}

// sweepInterval bounds how often popLoop runs promoteDue/reclaimStale: an
// idle Pop otherwise sends both Lua scripts to Redis on every poll tick
// (every blockTimeout, ~100ms), which is unnecessary overhead when nothing
// is due or stale. Gating the sweep to this cadence trades up to
// sweepInterval of extra recovery latency for a due/stale message for far
// fewer idle round trips.
const sweepInterval = 250 * time.Millisecond

type redisAdapter struct {
	client            redisClient
	rawClient         *goredis.Client
	prefix            string
	visibilityTimeout time.Duration
	pollTimeout       time.Duration
	buffer            int
	lastSweep         atomic.Int64
	closed            atomic.Bool
}

// connOptions maps queue options onto the shared client options. A set URL
// takes precedence over Addr; both spellings connect.
func connOptions(opts queue.Options) zredis.Options {
	addr := strings.TrimSpace(opts.URL)
	if addr == "" {
		addr = opts.Addr
	}

	return zredis.Options{
		Addr:       addr,
		Password:   opts.Password,
		DB:         opts.DB,
		TLS:        opts.TLS,
		RequireTLS: opts.RequireTLS,
	}
}

// New creates a Redis-backed queue adapter delegated via internal/redis with defaults of prefix "queue", VisibilityTimeout 30s, and PollTimeout 5s when unset. It verifies connectivity with a 3s ping check.
func New(opts queue.Options) (queue.Queue, error) {
	prefix := strings.TrimSpace(opts.Prefix)
	if prefix == "" {
		prefix = "queue"
	}

	visibility := opts.VisibilityTimeout
	if visibility <= 0 {
		visibility = DefaultVisibilityTimeout
	}

	pollTimeout := opts.PollTimeout
	if pollTimeout <= 0 {
		pollTimeout = DefaultPollTimeout
	}

	buf := opts.Buffer

	client, err := zredis.New(connOptions(opts))
	if err != nil {
		return nil, fmt.Errorf("queue: connect %q: %w", redactURL(opts), err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), DefaultPingTimeout)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()

		return nil, fmt.Errorf("queue: ping %q error: %w", redactURL(opts), err)
	}

	return &redisAdapter{
		client:            client,
		rawClient:         client,
		prefix:            prefix,
		visibilityTimeout: visibility,
		pollTimeout:       pollTimeout,
		buffer:            buf,
	}, nil
}

// redactURL returns opts.URL (or opts.Addr, if URL is empty) with any
// embedded userinfo credentials masked, suitable for inclusion in error
// messages.
func redactURL(opts queue.Options) string {
	return zredis.RedactEndpoint(opts.URL, opts.Addr)
}

func (a *redisAdapter) Push(ctx context.Context, topic string, payload queue.Payload, headers queue.Headers) error {
	if a.closed.Load() {
		return queue.ErrClosed
	}

	if a.buffer > 0 {
		if err := a.waitForSpace(ctx, topic); err != nil {
			return err
		}
	}

	msg := toWireMessage(queue.NewMessage(topic, payload, headers))

	b, err := jsonMarshal(msg)
	if err != nil {
		return fmt.Errorf("queue: marshal error: %w", err)
	}

	if err := a.client.RPush(ctx, a.readyKey(topic), b).Err(); err != nil {
		return fmt.Errorf("queue: push %q: %w", topic, err)
	}

	return nil
}

func (a *redisAdapter) PushDelayed(ctx context.Context, topic string, payload queue.Payload, headers queue.Headers, delay time.Duration) error {
	if a.closed.Load() {
		return queue.ErrClosed
	}

	if a.buffer > 0 {
		if delay <= 0 {
			if err := a.waitForSpace(ctx, topic); err != nil {
				return err
			}
		} else if err := a.waitForDelayedSpace(ctx, topic); err != nil {
			return err
		}
	}

	msg := toWireMessage(queue.NewMessage(topic, payload, headers))

	b, err := jsonMarshal(msg)
	if err != nil {
		return fmt.Errorf("queue: marshal message error: %w", err)
	}

	if delay <= 0 {
		if err := a.client.RPush(ctx, a.readyKey(topic), b).Err(); err != nil {
			return fmt.Errorf("queue: push %q: %w", topic, err)
		}

		return nil
	}

	if err := a.client.ZAdd(ctx, a.delayedKey(topic), goredis.Z{
		Score:  float64(time.Now().Add(delay).UnixMilli()),
		Member: b,
	}).Err(); err != nil {
		return fmt.Errorf("queue: push %q: %w", topic, err)
	}

	return nil
}

func (a *redisAdapter) Pop(ctx context.Context, topic string) (queue.Message, error) {
	if a.closed.Load() {
		return queue.Message{}, queue.ErrClosed
	}

	readyKey := a.readyKey(topic)
	processingKey := a.processingKey(topic)
	deadlineKey := a.deadlineKey(topic)

	poll := time.NewTimer(a.pollTimeout)
	defer poll.Stop()

	blockTimeout := DefaultBlockTimeout
	if blockTimeout > a.pollTimeout {
		blockTimeout = a.pollTimeout
	}

	raw, err := a.popLoop(ctx, topic, readyKey, processingKey, deadlineKey, poll, blockTimeout)
	if err != nil {
		return queue.Message{}, err
	}

	return decodeMessage(raw, topic)
}

func (a *redisAdapter) Ack(ctx context.Context, msg queue.Message) error {
	if a.closed.Load() {
		return queue.ErrClosed
	}

	processingKey := a.processingKey(msg.Topic)
	deadlineKey := a.deadlineKey(msg.Topic)

	_, err := ackScript.Run(ctx, a.client, []string{processingKey, deadlineKey}, msg.ID.String(), msg.Attempt).Int()
	if err != nil {
		return fmt.Errorf("queue: ack: %w", err)
	}

	return nil
}

func (a *redisAdapter) Nack(ctx context.Context, msg queue.Message, requeue bool) error {
	if a.closed.Load() {
		return queue.ErrClosed
	}

	processingKey := a.processingKey(msg.Topic)
	readyKey := a.readyKey(msg.Topic)
	deadlineKey := a.deadlineKey(msg.Topic)

	requeueFlag := 0
	if requeue {
		requeueFlag = 1
	}

	_, err := nackScript.Run(ctx, a.client,
		[]string{processingKey, readyKey, deadlineKey},
		msg.ID.String(), msg.Attempt, requeueFlag,
	).Int()
	if err != nil {
		return fmt.Errorf("queue: nack: %w", err)
	}

	return nil
}

func (a *redisAdapter) Length(ctx context.Context, topic string) (int64, error) {
	if a.closed.Load() {
		return 0, queue.ErrClosed
	}

	n, err := a.client.LLen(ctx, a.readyKey(topic)).Result()
	if err != nil {
		return 0, fmt.Errorf("queue: length: %w", err)
	}

	return n, nil
}

func (a *redisAdapter) IsEmpty(ctx context.Context, topic string) (bool, error) {
	n, err := a.Length(ctx, topic)
	return n == 0, err
}

func (a *redisAdapter) Close() error {
	if !a.closed.CompareAndSwap(false, true) {
		return nil
	}

	return zredis.Close(a.rawClient)
}

func (a *redisAdapter) Name() string { return "redis" }

func (a *redisAdapter) waitForSpace(ctx context.Context, topic string) error {
	return a.waitForBuffer(ctx, func(ctx context.Context) (int64, error) {
		return a.client.LLen(ctx, a.readyKey(topic)).Result()
	})
}

func (a *redisAdapter) waitForBuffer(ctx context.Context, count func(context.Context) (int64, error)) error {
	timer := time.NewTimer(bufferBackoffPolicy.BaseDelay)
	defer timer.Stop()
	timer.Stop()

	attempt := 0

	for {
		n, err := count(ctx)
		if err != nil {
			return fmt.Errorf("queue: check buffer capacity error: %w", err)
		}
		if n < int64(a.buffer) {
			return nil
		}

		attempt++

		timer.Stop()

		timer.Reset(nextBackoff(attempt))

		select {
		case <-ctx.Done():
			return fmt.Errorf("queue: failed to wait buffer space: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

// nextBackoff returns the poll delay before the given attempt (1-based),
// per bufferBackoffPolicy. NextDelay's additive jitter is applied after its
// own internal cap, so it can push the result above MaxDelay; re-clamp to
// preserve the exact floor/cap guarantees the poll loop depends on.
func nextBackoff(attempt int) time.Duration {
	d := bufferBackoffPolicy.NextDelay(attempt)
	if d < bufferBackoffPolicy.BaseDelay {
		d = bufferBackoffPolicy.BaseDelay
	}

	if d > bufferBackoffPolicy.MaxDelay {
		d = bufferBackoffPolicy.MaxDelay
	}

	return d
}

func (a *redisAdapter) popLoop(
	ctx context.Context,
	topic, readyKey, processingKey, deadlineKey string,
	poll *time.Timer,
	blockTimeout time.Duration,
) (string, error) {
	for {
		nowMillis := time.Now().UnixMilli()
		now := strconv.FormatInt(nowMillis, 10)

		if last := a.lastSweep.Load(); nowMillis-last >= sweepInterval.Milliseconds() {
			if a.lastSweep.CompareAndSwap(last, nowMillis) {
				if err := a.promoteDue(ctx, topic); err != nil {
					return "", err
				}

				if err := a.reclaimStale(ctx, topic); err != nil {
					return "", err
				}
			}
		}

		raw, claimed, err := a.tryClaim(ctx, readyKey, processingKey, deadlineKey, now)
		if err != nil {
			return "", err
		}

		if claimed {
			return raw, nil
		}

		raw, claimed, err = a.blockingClaim(ctx, readyKey, processingKey, deadlineKey, now, poll, blockTimeout)
		if err != nil {
			return "", err
		}

		if claimed {
			return raw, nil
		}
	}
}

func (a *redisAdapter) blockingClaim(
	ctx context.Context,
	readyKey, processingKey, deadlineKey, now string,
	poll *time.Timer,
	blockTimeout time.Duration,
) (string, bool, error) {
	val, err := a.client.BLPop(ctx, blockTimeout, readyKey).Result()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			select {
			case <-ctx.Done():
				return "", false, fmt.Errorf("queue: pop cancelled: %w", ctx.Err())
			case <-poll.C:
				return "", false, &queue.EmptyError{Topic: ""}
			default:
				return "", false, nil
			}
		}

		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", false, fmt.Errorf("queue: pop cancelled: %w", ctxErr)
		}

		return "", false, fmt.Errorf("queue: pop: %w", err)
	}

	if len(val) < 2 {
		return "", false, nil
	}

	raw := val[1]

	var wm wireMessage
	if unmarshalErr := jsonUnmarshal([]byte(raw), &wm); unmarshalErr != nil {
		return "", false, fmt.Errorf("queue: decode message: %w", unmarshalErr)
	}

	milis, parseErr := strconv.ParseInt(now, 10, 64)
	if parseErr != nil {
		milis = time.Now().UnixMilli()
	}

	wm.PoppedAt = milis

	enc, err := jsonMarshal(wm)
	if err != nil {
		_ = a.client.RPush(ctx, readyKey, raw).Err()

		return "", false, fmt.Errorf("queue: marshal error: %w", err)
	}

	att := strconv.Itoa(wm.Attempt)
	buf := make([]byte, 0, len(wm.ID)+1+len(att))
	buf = append(buf, wm.ID...)
	buf = append(buf, ':')
	buf = append(buf, att...)

	field := string(buf)

	if err := a.client.HSet(ctx, processingKey, field, enc).Err(); err != nil {
		_ = a.client.RPush(ctx, readyKey, raw).Err()

		return "", false, fmt.Errorf("queue: store claim: %w", err)
	}

	if err := a.client.ZAdd(ctx, deadlineKey, goredis.Z{Score: float64(milis), Member: field}).Err(); err != nil {
		_ = a.client.HDel(ctx, processingKey, field).Err()
		_ = a.client.RPush(ctx, readyKey, raw).Err()

		return "", false, fmt.Errorf("queue: store claim: %w", err)
	}

	return string(enc), true, nil
}

const promoteBatch = 100

func (a *redisAdapter) promoteDue(ctx context.Context, topic string) error {
	now := strconv.FormatInt(time.Now().UnixMilli(), 10)

	for {
		n, err := promoteScript.Run(
			ctx,
			a.client,
			[]string{a.delayedKey(topic), a.readyKey(topic)},
			now,
			promoteBatch,
		).Int()
		if err != nil {
			return fmt.Errorf("queue: promote due messages: %w", err)
		}

		if n == 0 {
			return nil
		}
	}
}

func decodeMessage(raw, topic string) (queue.Message, error) {
	var wm wireMessage
	if err := jsonUnmarshal([]byte(raw), &wm); err != nil {
		return queue.Message{}, fmt.Errorf("queue: decode message error: %w", err)
	}

	id, err := queue.ParseMessageID(wm.ID)
	if err != nil {
		return queue.Message{}, err
	}

	return queue.Message{
		ID:      id,
		Topic:   topic,
		Payload: wm.Payload.Clone(),
		Headers: wm.Headers.Clone(),
		Attempt: wm.Attempt,
	}, nil
}

func (a *redisAdapter) waitForDelayedSpace(ctx context.Context, topic string) error {
	return a.waitForBuffer(ctx, func(ctx context.Context) (int64, error) {
		ready, err := a.client.LLen(ctx, a.readyKey(topic)).Result()
		if err != nil {
			return 0, err
		}

		delayed, err := a.client.ZCard(ctx, a.delayedKey(topic)).Result()
		if err != nil {
			return 0, err
		}

		return ready + delayed, nil
	})
}

const reclaimBatch = 100

func (a *redisAdapter) reclaimStale(ctx context.Context, topic string) error {
	cutoff := strconv.FormatInt(time.Now().Add(-a.visibilityTimeout).UnixMilli(), 10)

	moved, err := reclaimScript.Run(ctx, a.client,
		[]string{a.processingKey(topic), a.readyKey(topic), a.deadlineKey(topic)},
		cutoff, reclaimBatch,
	).Int()
	if err != nil {
		return fmt.Errorf("queue: reclaim stale messages: %w", err)
	}

	if moved == 0 {
		return a.reclaimFallbackIfNeeded(ctx, topic, cutoff)
	}

	for moved == reclaimBatch {
		moved, err = reclaimScript.Run(ctx, a.client,
			[]string{a.processingKey(topic), a.readyKey(topic), a.deadlineKey(topic)},
			cutoff, reclaimBatch,
		).Int()
		if err != nil {
			return fmt.Errorf("queue: reclaim stale messages: %w", err)
		}

		if moved == 0 {
			break
		}
	}

	return nil
}

func (a *redisAdapter) reclaimFallbackIfNeeded(ctx context.Context, topic, cutoff string) error {
	n, err := a.client.ZCard(ctx, a.deadlineKey(topic)).Result()
	if err != nil {
		return fmt.Errorf("queue: reclaim stale messages: %w", err)
	}

	if n != 0 {
		return nil
	}

	hlen, err := a.client.HLen(ctx, a.processingKey(topic)).Result()
	if err != nil {
		return fmt.Errorf("queue: reclaim stale messages: %w", err)
	}

	if hlen == 0 {
		return nil
	}

	_, err = reclaimFallbackScript.Run(ctx, a.client,
		[]string{a.processingKey(topic), a.readyKey(topic), a.deadlineKey(topic)},
		cutoff,
	).Int()
	if err != nil {
		return fmt.Errorf("queue: reclaim stale messages: %w", err)
	}

	return nil
}

func (a *redisAdapter) tryClaim(ctx context.Context, readyKey, processingKey, deadlineKey, now string) (string, bool, error) {
	cmd := claimScript.Run(ctx, a.client, []string{readyKey, processingKey, deadlineKey}, now)
	if err := cmd.Err(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", false, ctxErr
		}

		return "", false, fmt.Errorf("queue: pop: %w", err)
	}

	claimed, ok := cmd.Val().(string)
	if ok {
		return claimed, true, nil
	}

	return "", false, nil
}
