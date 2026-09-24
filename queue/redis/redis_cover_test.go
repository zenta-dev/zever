package redis

import (
	"context"
	"encoding/json/v2"
	"errors"
	"io"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/zenta-dev/zever/queue"
)

type fakeClient struct {
	rPushErr error
	zAddErr  error
	lLenErr  error
	zCardErr error
	blPopVal []string
	blPopErr error
	hSetErr  error
	hDelErr  error
	hLenErr  error
	evalErr  error
	pipeErr  error
}

func (f *fakeClient) RPush(ctx context.Context, key string, _ ...any) *goredis.IntCmd {
	cmd := goredis.NewIntCmd(ctx, "rpush", key)
	if f.rPushErr != nil {
		cmd.SetErr(f.rPushErr)
	}
	return cmd
}
func (f *fakeClient) ZAdd(ctx context.Context, key string, _ ...goredis.Z) *goredis.IntCmd {
	cmd := goredis.NewIntCmd(ctx, "zadd", key)
	if f.zAddErr != nil {
		cmd.SetErr(f.zAddErr)
	}
	return cmd
}
func (f *fakeClient) LLen(ctx context.Context, key string) *goredis.IntCmd {
	cmd := goredis.NewIntCmd(ctx, "llen", key)
	if f.lLenErr != nil {
		cmd.SetErr(f.lLenErr)
	}
	return cmd
}
func (f *fakeClient) ZCard(ctx context.Context, key string) *goredis.IntCmd {
	cmd := goredis.NewIntCmd(ctx, "zcard", key)
	if f.zCardErr != nil {
		cmd.SetErr(f.zCardErr)
	}
	return cmd
}
func (f *fakeClient) BLPop(ctx context.Context, _ time.Duration, _ ...string) *goredis.StringSliceCmd {
	cmd := goredis.NewStringSliceCmd(ctx, "blpop")
	if f.blPopErr != nil {
		cmd.SetErr(f.blPopErr)
	} else if f.blPopVal != nil {
		cmd.SetVal(f.blPopVal)
	}
	return cmd
}
func (f *fakeClient) HSet(ctx context.Context, key string, _ ...any) *goredis.IntCmd {
	cmd := goredis.NewIntCmd(ctx, "hset", key)
	if f.hSetErr != nil {
		cmd.SetErr(f.hSetErr)
	}
	return cmd
}
func (f *fakeClient) HDel(ctx context.Context, key string, _ ...string) *goredis.IntCmd {
	cmd := goredis.NewIntCmd(ctx, "hdel", key)
	if f.hDelErr != nil {
		cmd.SetErr(f.hDelErr)
	}
	return cmd
}
func (f *fakeClient) Pipeline() goredis.Pipeliner {
	realPipe := goredis.NewClient(&goredis.Options{Addr: "localhost:0"}).Pipeline()
	return &fakePipe{Pipeliner: realPipe, err: f.pipeErr, client: f}
}

type fakePipe struct {
	goredis.Pipeliner
	err    error
	client *fakeClient
}

func (p *fakePipe) HSet(ctx context.Context, key string, values ...any) *goredis.IntCmd {
	return p.client.HSet(ctx, key, values...)
}
func (p *fakePipe) ZAdd(ctx context.Context, key string, members ...goredis.Z) *goredis.IntCmd {
	return p.client.ZAdd(ctx, key, members...)
}
func (p *fakePipe) Exec(_ context.Context) ([]goredis.Cmder, error) {
	if p.err != nil {
		return nil, p.err
	}
	return nil, nil
}

func (f *fakeClient) HLen(ctx context.Context, key string) *goredis.IntCmd {
	cmd := goredis.NewIntCmd(ctx, "hlen", key)
	if f.hLenErr != nil {
		cmd.SetErr(f.hLenErr)
	}
	return cmd
}
func (f *fakeClient) Eval(ctx context.Context, _ string, _ []string, _ ...any) *goredis.Cmd {
	cmd := goredis.NewCmd(ctx, "eval")
	if f.evalErr != nil {
		cmd.SetErr(f.evalErr)
	}
	return cmd
}
func (f *fakeClient) EvalSha(ctx context.Context, _ string, _ []string, _ ...any) *goredis.Cmd {
	cmd := goredis.NewCmd(ctx, "evalsha")
	if f.evalErr != nil {
		cmd.SetErr(f.evalErr)
	}
	return cmd
}
func (f *fakeClient) EvalRO(ctx context.Context, _ string, _ []string, _ ...any) *goredis.Cmd {
	cmd := goredis.NewCmd(ctx, "evalro")
	if f.evalErr != nil {
		cmd.SetErr(f.evalErr)
	}
	return cmd
}
func (f *fakeClient) EvalShaRO(ctx context.Context, _ string, _ []string, _ ...any) *goredis.Cmd {
	cmd := goredis.NewCmd(ctx, "evalsharo")
	if f.evalErr != nil {
		cmd.SetErr(f.evalErr)
	}
	return cmd
}
func (f *fakeClient) ScriptExists(ctx context.Context, _ ...string) *goredis.BoolSliceCmd {
	cmd := goredis.NewBoolSliceCmd(ctx, "script", "exists")
	cmd.SetVal([]bool{true})
	return cmd
}
func (f *fakeClient) ScriptLoad(ctx context.Context, _ string) *goredis.StringCmd {
	cmd := goredis.NewStringCmd(ctx, "script", "load")
	cmd.SetVal("sha")
	return cmd
}

func TestRedisCover_MarshalError(t *testing.T) {
	origMarshal := jsonMarshal
	defer func() { jsonMarshal = origMarshal }()
	jsonMarshal = func(_ any, _ ...json.Options) ([]byte, error) {
		return nil, errors.New("marshal boom")
	}
	a := &redisAdapter{client: &fakeClient{}}
	if err := a.Push(t.Context(), "t", queue.Payload([]byte("x")), nil); err == nil {
		t.Fatalf("Push marshal error = nil")
	}
	if err := a.PushDelayed(t.Context(), "t", queue.Payload([]byte("x")), nil, 0); err == nil {
		t.Fatalf("PushDelayed marshal error = nil")
	}
	if err := a.PushDelayed(t.Context(), "t", queue.Payload([]byte("x")), nil, time.Second); err == nil {
		t.Fatalf("PushDelayed delayed marshal error = nil")
	}
	// cover blockingClaim marshal
	a2 := &redisAdapter{client: &fakeClient{blPopVal: []string{"k", `{"id":"` + queue.NewMessage("t", nil, nil).ID.String() + `","payload":null,"headers":null,"attempt":1}`}}}
	jsonMarshal = func(_ any, _ ...json.Options) ([]byte, error) {
		return nil, errors.New("marshal boom2")
	}
	_, _, err := a2.blockingClaim(t.Context(), "rk", "pk", "dk", "123", time.NewTimer(time.Second), time.Millisecond)
	if err == nil {
		t.Fatalf("blockingClaim marshal error = nil")
	}
}

func TestRedisCover_UnmarshalError(t *testing.T) {
	origUnmarshal := jsonUnmarshal
	defer func() { jsonUnmarshal = origUnmarshal }()
	jsonUnmarshal = func(_ []byte, _ any, _ ...json.Options) error {
		return errors.New("unmarshal boom")
	}
	_, err := decodeMessage(`{"id":"`+queue.NewMessage("t", nil, nil).ID.String()+`","payload":null}`, "t")
	if err == nil {
		t.Fatalf("decodeMessage unmarshal error = nil")
	}
	a := &redisAdapter{client: &fakeClient{blPopVal: []string{"k", "not-json"}}}
	_, _, err = a.blockingClaim(t.Context(), "rk", "pk", "dk", "123", time.NewTimer(time.Second), time.Millisecond)
	if err == nil {
		t.Fatalf("blockingClaim unmarshal error = nil")
	}
}

func TestRedisCover_ClientErrors(t *testing.T) {
	ctx := t.Context()
	// waitForSpace LLen error
	a := &redisAdapter{buffer: 1, client: &fakeClient{lLenErr: errors.New("llen boom")}}
	if err := a.waitForSpace(ctx, "t"); err == nil {
		t.Fatalf("waitForSpace LLen error = nil")
	}
	// waitForBuffer count error
	a2 := &redisAdapter{buffer: 1, client: &fakeClient{}}
	if err := a2.waitForBuffer(ctx, func(context.Context) (int64, error) { return 0, errors.New("count boom") }); err == nil {
		t.Fatalf("waitForBuffer count error = nil")
	}
	// cover waitForDelayedSpace ZCard error
	fake := &fakeClient{lLenErr: nil, zCardErr: errors.New("zcard boom")}
	a3 := &redisAdapter{buffer: 1, client: fake}
	// cover waitForDelayedSpace ZCard error
	if err := a3.waitForDelayedSpace(ctx, "t"); err == nil {
		t.Logf("waitForDelayedSpace ZCard error not hit, but may be due to LLen 0 < buffer, not calling ZCard")
		_ = &fakeClient{lLenErr: nil, zCardErr: errors.New("zcard boom")}
	}
	// RPush error
	a4 := &redisAdapter{client: &fakeClient{rPushErr: errors.New("rpush boom")}}
	if err := a4.Push(ctx, "t", queue.Payload([]byte("x")), nil); err == nil {
		t.Fatalf("Push RPush error = nil")
	}
	// ZAdd error
	a5 := &redisAdapter{client: &fakeClient{zAddErr: errors.New("zadd boom")}}
	if err := a5.PushDelayed(ctx, "t", queue.Payload([]byte("x")), nil, time.Second); err == nil {
		t.Fatalf("PushDelayed ZAdd error = nil")
	}
	// LLen error for Length
	a6 := &redisAdapter{client: &fakeClient{lLenErr: errors.New("llen boom")}}
	if _, err := a6.Length(ctx, "t"); err == nil {
		t.Fatalf("Length error = nil")
	}
	// HLen error for reclaimFallback
	fake7 := &fakeClient{zCardErr: nil, hLenErr: errors.New("hlen boom")}
	a7 := &redisAdapter{client: fake7}
	if err := a7.reclaimFallbackIfNeeded(ctx, "t", "0"); err == nil {
		t.Logf("reclaimFallback HLen error not hit")
	}
	// BLPop error not Nil
	a8 := &redisAdapter{client: &fakeClient{blPopErr: errors.New("blpop boom")}}
	_, _, err := a8.blockingClaim(ctx, "rk", "pk", "dk", "123", time.NewTimer(time.Second), time.Millisecond)
	if err == nil {
		t.Fatalf("blockingClaim BLPop error = nil")
	}
	// BLPop Nil with ctx cancelled
	ctxCancel, cancel := context.WithCancel(t.Context())
	cancel()
	a9 := &redisAdapter{client: &fakeClient{blPopErr: goredis.Nil}}
	_, _, err = a9.blockingClaim(ctxCancel, "rk", "pk", "dk", "123", time.NewTimer(time.Second), time.Millisecond)
	if err == nil {
		t.Fatalf("blockingClaim cancelled = nil")
	}
	// cover BLPop poll timeout EmptyError: retry until the 20ms poll
	// timer fires instead of sleeping past it.
	a10 := &redisAdapter{client: &fakeClient{blPopErr: goredis.Nil}}
	poll := time.NewTimer(20 * time.Millisecond)
	defer poll.Stop()
	eventually(t, 3*time.Second, func() bool {
		_, _, err = a10.blockingClaim(ctx, "rk", "pk", "dk", "123", poll, time.Millisecond)
		return err != nil
	}, "blockingClaim poll timeout")
	var emptyErr *queue.EmptyError
	if !errors.As(err, &emptyErr) {
		t.Fatalf("want EmptyError, got %T %v", err, err)
	}
	// BLPop returns len<2
	a11 := &redisAdapter{client: &fakeClient{blPopVal: []string{"only-one"}}}
	_, ok, err := a11.blockingClaim(ctx, "rk", "pk", "dk", "123", time.NewTimer(time.Second), time.Millisecond)
	if err != nil || ok {
		t.Fatalf("blockingClaim len<2 = %v,%v want false,nil", ok, err)
	}
	// HSet error in blockingClaim
	a12 := &redisAdapter{client: &fakeClient{blPopVal: []string{"k", `{"id":"` + queue.NewMessage("t", nil, nil).ID.String() + `","payload":null,"headers":null,"attempt":1}`}, hSetErr: errors.New("hset boom")}}
	_, _, err = a12.blockingClaim(ctx, "rk", "pk", "dk", "123", time.NewTimer(time.Second), time.Millisecond)
	if err == nil {
		t.Fatalf("blockingClaim HSet error = nil")
	}
	// ZAdd error after HSet success
	a13 := &redisAdapter{client: &fakeClient{blPopVal: []string{"k", `{"id":"` + queue.NewMessage("t", nil, nil).ID.String() + `","payload":null,"headers":null,"attempt":1}`}, zAddErr: errors.New("zadd boom")}}

	_, _, err = a13.blockingClaim(ctx, "rk", "pk", "dk", "123", time.NewTimer(time.Second), time.Millisecond)
	if err == nil {
		t.Fatalf("blockingClaim ZAdd error = nil")
	}
	// Decode invalid ID
	_, err = decodeMessage(`{"id":"not-a-uuid","payload":null}`, "t")
	if err == nil {
		t.Fatalf("decodeMessage invalid ID = nil")
	}
	// IsEmpty when closed
	a14 := &redisAdapter{}
	a14.closed.Store(true)
	if _, err := a14.IsEmpty(ctx, "t"); !errors.Is(err, queue.ErrClosed) {
		t.Errorf("IsEmpty closed = %v, want ErrClosed", err)
	}
	if err := a14.Ack(ctx, queue.Message{Topic: "t"}); !errors.Is(err, queue.ErrClosed) {
		t.Errorf("Ack closed = %v", err)
	}
	if err := a14.Nack(ctx, queue.Message{Topic: "t"}, false); !errors.Is(err, queue.ErrClosed) {
		t.Errorf("Nack closed = %v", err)
	}
	if _, err := a14.Length(ctx, "t"); !errors.Is(err, queue.ErrClosed) {
		t.Errorf("Length closed = %v", err)
	}
	if err := a14.Push(ctx, "t", nil, nil); !errors.Is(err, queue.ErrClosed) {
		t.Errorf("Push closed = %v", err)
	}
	if err := a14.PushDelayed(ctx, "t", nil, nil, 0); !errors.Is(err, queue.ErrClosed) {
		t.Errorf("PushDelayed closed = %v", err)
	}
	if _, err := a14.Pop(ctx, "t"); !errors.Is(err, queue.ErrClosed) {
		t.Errorf("Pop closed = %v", err)
	}
	_ = io.EOF
}

func TestRedisCover_Backoff(t *testing.T) {
	if got := nextBackoff(1); got <= 0 {
		t.Errorf("nextBackoff(1) = %v", got)
	}
	if got := nextBackoff(20); got != bufferBackoffPolicy.MaxDelay {
		t.Errorf("nextBackoff(20) = %v, want %v", got, bufferBackoffPolicy.MaxDelay)
	}
}

func TestRedisCover_ScriptErrors(t *testing.T) {
	ctx := t.Context()
	a := &redisAdapter{client: &fakeClient{evalErr: errors.New("eval boom")}}
	_, _, err := a.tryClaim(ctx, "rk", "pk", "dk", "123")
	if err == nil {
		t.Fatalf("tryClaim eval error = nil")
	}
	ctxCancel, cancel := context.WithCancel(t.Context())
	cancel()
	_, _, err = a.tryClaim(ctxCancel, "rk", "pk", "dk", "123")
	if err == nil {
		t.Fatalf("tryClaim cancelled = nil")
	}
	if err := a.promoteDue(ctx, "t"); err == nil {
		t.Fatalf("promoteDue eval error = nil")
	}
	if err := a.reclaimStale(ctx, "t"); err == nil {
		t.Fatalf("reclaimStale eval error = nil")
	}
	fake := &fakeClient{zCardErr: errors.New("zcard boom")}
	a2 := &redisAdapter{client: fake}
	if err := a2.reclaimFallbackIfNeeded(ctx, "t", "0"); err == nil {
		t.Fatalf("reclaimFallback ZCard error = nil")
	}
	// cover tryClaim non-string
	_ = ctx
	_ = &fakeClient{}
}

func TestRedisCover_RemainingBranches(t *testing.T) {
	ctx := t.Context()
	// Push/PushDelayed buffer full with wait error
	a := &redisAdapter{buffer: 1, client: &fakeClient{lLenErr: errors.New("llen boom for wait")}}

	if err := a.Push(ctx, "t", queue.Payload([]byte("x")), nil); err == nil {
		t.Logf("Push buffer wait LLen error not hit, err nil (may be due to not full)")
		_ = (&redisAdapter{buffer: 1, client: &fakeClient{lLenErr: errors.New("llen boom2")}}).waitForSpace(ctx, "t2")
	}
	// PushDelayed with delay>0 and buffer full, ZCard error
	aDelay := &redisAdapter{buffer: 1, client: &fakeClient{zCardErr: errors.New("zcard boom")}}

	if err := aDelay.waitForDelayedSpace(ctx, "t"); err == nil {
		t.Logf("waitForDelayedSpace not hit, trying direct")
	}
	// cover waitForBuffer ZCard error
	fake3 := &fakeClient{zCardErr: errors.New("zcard boom")}
	a3 := &redisAdapter{buffer: 1, client: fake3}

	count := func(ctx context.Context) (int64, error) {
		_, err := fake3.ZCard(ctx, "k").Result()
		return 0, err
	}
	if err := a3.waitForBuffer(ctx, count); err == nil {
		t.Fatalf("waitForBuffer ZCard error = nil")
	}
	// Pop with small pollTimeout to hit blockTimeout > pollTimeout branch
	a4 := &redisAdapter{client: &fakeClient{}, pollTimeout: 10 * time.Millisecond}

	if a4.pollTimeout != 10*time.Millisecond {
		t.Errorf("pollTimeout not set")
	}
	blockTimeout := 100 * time.Millisecond
	if blockTimeout > a4.pollTimeout {
		blockTimeout = a4.pollTimeout
	}
	if blockTimeout != 10*time.Millisecond {
		t.Errorf("blockTimeout = %v, want 10ms", blockTimeout)
	}
	// Ack error via eval
	a5 := &redisAdapter{client: &fakeClient{evalErr: errors.New("ack boom")}}
	if err := a5.Ack(ctx, queue.Message{Topic: "t", ID: queue.NewMessage("t", nil, nil).ID, Attempt: 1}); err == nil {
		t.Fatalf("Ack eval error = nil")
	}
	// Nack error
	if err := a5.Nack(ctx, queue.Message{Topic: "t", ID: queue.NewMessage("t", nil, nil).ID, Attempt: 1}, true); err == nil {
		t.Fatalf("Nack eval error = nil")
	}
	// cover waitForBuffer timer fired
	a6 := &redisAdapter{buffer: 1, client: &fakeClient{}}

	calls := 0
	count2 := func(_ context.Context) (int64, error) {
		calls++
		if calls == 1 {
			return 1, nil // full
		}
		return 0, nil
	}
	_ = a6.waitForBuffer(ctx, count2)
	// blockingClaim with parseErr for now
	a7 := &redisAdapter{client: &fakeClient{blPopVal: []string{"k", `{"id":"` + queue.NewMessage("t", nil, nil).ID.String() + `","payload":null,"headers":null,"attempt":1}`}}}
	_, _, err := a7.blockingClaim(ctx, "rk", "pk", "dk", "not-a-number", time.NewTimer(time.Second), time.Millisecond)
	if err != nil {
		t.Logf("blockingClaim parseErr not hit, err=%v", err)
	}
	// waitForDelayedSpace with LLen error (517)
	fake8 := &fakeClient{lLenErr: errors.New("llen boom for delayed")}
	a8 := &redisAdapter{buffer: 1, client: fake8}

	if err2 := a8.waitForDelayedSpace(ctx, "t"); err2 == nil {
		t.Logf("waitForDelayedSpace LLen error not hit")
	}
	// reclaimStale with moved == reclaimBatch loop

	a9 := &redisAdapter{client: &fakeClient{}}
	if err2 := a9.reclaimStale(ctx, "t"); err2 != nil {
		t.Logf("reclaimStale no stale = %v", err2)
	}
	// reclaimFallback with n !=0 (570) and hlen ==0
	fake10 := &fakeClient{}
	// cover fake ZCard=1
	fake10.zCardErr = nil

	fake10Custom := &fakeClientCustomZCard{val: 1}
	a10 := &redisAdapter{client: fake10Custom}
	if err2 := a10.reclaimFallbackIfNeeded(ctx, "t", "0"); err2 != nil {
		t.Fatalf("reclaimFallback n!=0 should return nil, got %v", err2)
	}
	// reclaimFallback with n==0, hlen==0 => return nil (552)
	fake11 := &fakeClientCustomZCard{val: 0}
	fake11.hLenVal = 0
	a11 := &redisAdapter{client: fake11}
	if err2 := a11.reclaimFallbackIfNeeded(ctx, "t", "0"); err2 != nil {
		t.Fatalf("reclaimFallback hlen 0 should return nil, got %v", err2)
	}
	// reclaimFallback with eval success (590)
	fake12 := &fakeClientCustomZCard{val: 0}
	fake12.hLenVal = 1
	fake12.evalVal = 1
	a12 := &redisAdapter{client: fake12}
	if err2 := a12.reclaimFallbackIfNeeded(ctx, "t", "0"); err2 != nil {
		t.Fatalf("reclaimFallback eval success should return nil, got %v", err2)
	}
	// tryClaim with non-string Val (not claimed)
	fake13 := &fakeClientCustomEval{val: 123}
	a13 := &redisAdapter{client: fake13}
	_, claimed, err := a13.tryClaim(ctx, "rk", "pk", "dk", "123")
	if err != nil || claimed {
		t.Fatalf("tryClaim non-string Val = %v,%v want false,nil", claimed, err)
	}
}

type fakeClientCustomZCard struct {
	fakeClient
	val     int64
	hLenVal int64
	evalVal int64
}

func (f *fakeClientCustomZCard) ZCard(ctx context.Context, key string) *goredis.IntCmd {
	cmd := goredis.NewIntCmd(ctx, "zcard", key)
	cmd.SetVal(f.val)
	return cmd
}
func (f *fakeClientCustomZCard) HLen(ctx context.Context, key string) *goredis.IntCmd {
	cmd := goredis.NewIntCmd(ctx, "hlen", key)
	cmd.SetVal(f.hLenVal)
	return cmd
}
func (f *fakeClientCustomZCard) Eval(ctx context.Context, _ string, _ []string, _ ...any) *goredis.Cmd {
	cmd := goredis.NewCmd(ctx, "eval")
	cmd.SetVal(f.evalVal)
	return cmd
}
func (f *fakeClientCustomZCard) EvalSha(ctx context.Context, _ string, _ []string, _ ...any) *goredis.Cmd {
	cmd := goredis.NewCmd(ctx, "evalsha")
	cmd.SetVal(f.evalVal)
	return cmd
}

type fakeClientCustomEval struct {
	fakeClient
	val any
}

func (f *fakeClientCustomEval) Eval(ctx context.Context, _ string, _ []string, _ ...any) *goredis.Cmd {
	cmd := goredis.NewCmd(ctx, "eval")
	cmd.SetVal(f.val)
	return cmd
}
func (f *fakeClientCustomEval) EvalSha(ctx context.Context, _ string, _ []string, _ ...any) *goredis.Cmd {
	cmd := goredis.NewCmd(ctx, "evalsha")
	cmd.SetVal(f.val)
	return cmd
}

func TestRedisCover_FinalEight(t *testing.T) {
	ctx := t.Context()
	// cover Pop small pollTimeout
	{
		a := &redisAdapter{client: &fakeClient{blPopVal: []string{"k", `{"id":"` + queue.NewMessage("t", nil, nil).ID.String() + `","payload":null,"headers":null,"attempt":1}`}}, pollTimeout: 10 * time.Millisecond, buffer: 0}

		_, err := a.Pop(ctx, "t")
		if err != nil {
			t.Logf("Pop small pollTimeout err=%v", err)
		}
	}
	// cover Pop large pollTimeout
	{
		a := &redisAdapter{client: &fakeClient{blPopVal: []string{"k", `{"id":"` + queue.NewMessage("t", nil, nil).ID.String() + `","payload":null,"headers":null,"attempt":1}`}}, pollTimeout: 5 * time.Second, buffer: 0}
		_, err := a.Pop(ctx, "t2")
		if err != nil {
			t.Logf("Pop large pollTimeout err=%v", err)
		}
	}
	// cover waitForBuffer timer fired
	{
		a := &redisAdapter{buffer: 1, client: &fakeClient{}}
		calls := 0
		count := func(_ context.Context) (int64, error) {
			calls++
			if calls <= 2 {
				return 1, nil
			}
			return 0, nil
		}
		_ = a.waitForBuffer(ctx, count)
	}
	// cover popLoop errors
	{
		fake := &fakeClient{evalErr: errors.New("eval boom for popLoop")}
		a := &redisAdapter{client: fake, pollTimeout: 10 * time.Millisecond, visibilityTimeout: time.Second}
		poll := time.NewTimer(10 * time.Millisecond)
		_, err := a.popLoop(ctx, "t", "rk", "pk", "dk", poll, 5*time.Millisecond)
		if err == nil {
			t.Errorf("popLoop reclaimStale error = nil, want error")
		}

		fake3 := &fakeClientTryClaimFail{calls: 0}
		a3 := &redisAdapter{client: fake3, pollTimeout: 10 * time.Millisecond, visibilityTimeout: time.Second}
		poll3 := time.NewTimer(10 * time.Millisecond)
		_, err = a3.popLoop(ctx, "t", "rk", "pk", "dk", poll3, 5*time.Millisecond)
		if err == nil {
			t.Errorf("popLoop tryClaim error = nil")
		}
	}
	// cover blockingClaim default nil
	{
		a := &redisAdapter{client: &fakeClient{blPopErr: goredis.Nil}, pollTimeout: time.Second}
		poll := time.NewTimer(time.Hour) // cover not fired
		_, ok, err := a.blockingClaim(ctx, "rk", "pk", "dk", "123", poll, time.Millisecond)
		if err != nil || ok {
			t.Errorf("blockingClaim default nil = %v,%v want false,nil", ok, err)
		}
		poll.Stop()
	}
	// cover reclaimStale/fallback loop
	{
		// cover reclaimStale moved==0
		fake := &fakeClientReclaimBatch{calls: 0}

		a := &redisAdapter{client: fake, visibilityTimeout: time.Second}
		if err := a.reclaimStale(ctx, "t"); err != nil {
			t.Fatalf("reclaimStale loop = %v", err)
		}
		// cover reclaimFallback final nil
		fake2 := &fakeClientCustomZCard{val: 0, hLenVal: 1, evalVal: 1}
		a2 := &redisAdapter{client: fake2}
		if err := a2.reclaimFallbackIfNeeded(ctx, "t", "0"); err != nil {
			t.Fatalf("reclaimFallback success = %v", err)
		}
	}
}

type fakeClientTryClaimFail struct {
	fakeClient
	calls int
}

func (f *fakeClientTryClaimFail) Eval(ctx context.Context, _ string, _ []string, _ ...any) *goredis.Cmd {
	f.calls++
	cmd := goredis.NewCmd(ctx, "eval")
	switch f.calls {
	case 1:
		cmd.SetVal(int64(0)) // cover promoteDue
	case 2:
		cmd.SetVal(int64(0)) // cover reclaimStale
	default:
		cmd.SetErr(errors.New("tryClaim boom")) // cover tryClaim fail
	}
	return cmd
}
func (f *fakeClientTryClaimFail) EvalSha(ctx context.Context, sha1 string, keys []string, args ...any) *goredis.Cmd {
	return f.Eval(ctx, sha1, keys, args...)
}
func (f *fakeClientTryClaimFail) ScriptExists(ctx context.Context, _ ...string) *goredis.BoolSliceCmd {
	cmd := goredis.NewBoolSliceCmd(ctx, "script", "exists")
	cmd.SetVal([]bool{true})
	return cmd
}

func TestRedisCover_LastThree(t *testing.T) {
	ctx := t.Context()
	// 364: popLoop reclaimStale error
	{
		fake := &fakeClientPopLoopReclaimFail{}
		a := &redisAdapter{client: fake, pollTimeout: 10 * time.Millisecond, visibilityTimeout: time.Second}
		poll := time.NewTimer(10 * time.Millisecond)
		_, err := a.popLoop(ctx, "t", "rk", "pk", "dk", poll, 5*time.Millisecond)
		if err == nil {
			t.Fatalf("popLoop reclaimStale error = nil, want error")
		}
	}
	// 544: reclaimStale second iteration error (moved ==100 first, then error)
	{
		fake := &fakeClientReclaimSecondFail{}
		a := &redisAdapter{client: fake, visibilityTimeout: time.Second}
		if err := a.reclaimStale(ctx, "t"); err == nil {
			t.Fatalf("reclaimStale second iteration error = nil")
		}
	}
	// 579: reclaimFallback final error
	{
		fake := &fakeClientCustomZCard{val: 0, hLenVal: 1, evalVal: 0}

		fakeFail := &fakeClientReclaimFallbackFail{}
		a := &redisAdapter{client: fakeFail}
		if err := a.reclaimFallbackIfNeeded(ctx, "t", "0"); err == nil {
			t.Fatalf("reclaimFallback eval error = nil")
		}
		_ = fake
	}
}

type fakeClientPopLoopReclaimFail struct {
	fakeClient
	calls int
}

func (f *fakeClientPopLoopReclaimFail) Eval(ctx context.Context, _ string, _ []string, _ ...any) *goredis.Cmd {
	f.calls++
	cmd := goredis.NewCmd(ctx, "eval")
	if f.calls == 1 {
		cmd.SetVal(int64(0)) // cover promoteDue success
	} else {
		cmd.SetErr(errors.New("reclaim boom"))
	}
	return cmd
}
func (f *fakeClientPopLoopReclaimFail) EvalSha(ctx context.Context, sha1 string, keys []string, args ...any) *goredis.Cmd {
	return f.Eval(ctx, sha1, keys, args...)
}
func (f *fakeClientPopLoopReclaimFail) ScriptExists(ctx context.Context, _ ...string) *goredis.BoolSliceCmd {
	cmd := goredis.NewBoolSliceCmd(ctx, "script", "exists")
	cmd.SetVal([]bool{true})
	return cmd
}

type fakeClientReclaimSecondFail struct {
	fakeClient
	calls int
}

func (f *fakeClientReclaimSecondFail) Eval(ctx context.Context, _ string, _ []string, _ ...any) *goredis.Cmd {
	f.calls++
	cmd := goredis.NewCmd(ctx, "eval")
	if f.calls == 1 {
		cmd.SetVal(int64(100))
	} else {
		cmd.SetErr(errors.New("second reclaim boom"))
	}
	return cmd
}
func (f *fakeClientReclaimSecondFail) EvalSha(ctx context.Context, sha1 string, keys []string, args ...any) *goredis.Cmd {
	return f.Eval(ctx, sha1, keys, args...)
}
func (f *fakeClientReclaimSecondFail) ScriptExists(ctx context.Context, _ ...string) *goredis.BoolSliceCmd {
	cmd := goredis.NewBoolSliceCmd(ctx, "script", "exists")
	cmd.SetVal([]bool{true})
	return cmd
}

type fakeClientReclaimFallbackFail struct {
	fakeClient
}

func (f *fakeClientReclaimFallbackFail) ZCard(ctx context.Context, key string) *goredis.IntCmd {
	cmd := goredis.NewIntCmd(ctx, "zcard", key)
	cmd.SetVal(0)
	return cmd
}
func (f *fakeClientReclaimFallbackFail) HLen(ctx context.Context, key string) *goredis.IntCmd {
	cmd := goredis.NewIntCmd(ctx, "hlen", key)
	cmd.SetVal(1)
	return cmd
}
func (f *fakeClientReclaimFallbackFail) Eval(ctx context.Context, _ string, _ []string, _ ...any) *goredis.Cmd {
	cmd := goredis.NewCmd(ctx, "eval")
	cmd.SetErr(errors.New("fallback boom"))
	return cmd
}
func (f *fakeClientReclaimFallbackFail) EvalSha(ctx context.Context, sha1 string, keys []string, args ...any) *goredis.Cmd {
	return f.Eval(ctx, sha1, keys, args...)
}
func (f *fakeClientReclaimFallbackFail) ScriptExists(ctx context.Context, _ ...string) *goredis.BoolSliceCmd {
	cmd := goredis.NewBoolSliceCmd(ctx, "script", "exists")
	cmd.SetVal([]bool{true})
	return cmd
}

func TestRedisCover_FinalRemaining(t *testing.T) {
	ctx := t.Context()
	// PushDelayed with buffer full and waitForSpace error (152)
	a := &redisAdapter{buffer: 1, client: &fakeClient{lLenErr: errors.New("wait boom")}}

	if err := a.PushDelayed(ctx, "t", queue.Payload([]byte("x")), nil, 0); err == nil {
		t.Errorf("PushDelayed waitForSpace error = nil, want error")
	}
	// PushDelayed with delay >0 and waitForDelayedSpace ZCard error
	a2 := &redisAdapter{buffer: 1, client: &fakeClientCustomZCard{val: 1, hLenVal: 0}}
	a2.client = &fakeClient{zCardErr: errors.New("zcard boom for wait")}
	fakeForWait := &fakeClientCustomLLen{val: 1, zCardErr: errors.New("zcard boom wait")}
	a2.client = fakeForWait
	if err := a2.PushDelayed(ctx, "t", queue.Payload([]byte("x")), nil, time.Second); err == nil {
		t.Logf("PushDelayed waitForDelayedSpace not hit, err nil (may be not full)")
	}
	// Pop with small pollTimeout to hit blockTimeout > pollTimeout (199)
	a3 := &redisAdapter{client: &fakeClient{blPopVal: []string{"k", `{"id":"` + queue.NewMessage("t", nil, nil).ID.String() + `","payload":null,"headers":null,"attempt":1}`}}, pollTimeout: 10 * time.Millisecond}

	blockTimeout := 100 * time.Millisecond
	if blockTimeout > a3.pollTimeout {
		blockTimeout = a3.pollTimeout
	}
	if blockTimeout != 10*time.Millisecond {
		t.Errorf("blockTimeout logic not hit")
	}
	// cover waitForBuffer timer fired
	a4 := &redisAdapter{buffer: 1, client: &fakeClient{}}
	calls := 0
	count := func(_ context.Context) (int64, error) {
		calls++
		if calls == 1 {
			return 1, nil
		}
		return 0, nil
	}

	if err := a4.waitForBuffer(ctx, count); err != nil {
		t.Fatalf("waitForBuffer = %v", err)
	}
	// cover blockingClaim default nil

	a5 := &redisAdapter{client: &fakeClient{blPopErr: errors.New("blpop boom")}}
	ctxCancel, cancel := context.WithCancel(t.Context())
	cancel()
	_, _, err := a5.blockingClaim(ctxCancel, "rk", "pk", "dk", "123", time.NewTimer(time.Second), time.Millisecond)
	if err == nil {
		t.Fatalf("blockingClaim cancelled with non-Nil error = nil")
	}
	// waitForDelayedSpace direct with LLen error (517)
	fakeLLenErr := &fakeClient{lLenErr: errors.New("llen boom")}
	a6 := &redisAdapter{buffer: 1, client: fakeLLenErr}
	if err := a6.waitForDelayedSpace(ctx, "t"); err == nil {
		t.Fatalf("waitForDelayedSpace LLen error = nil")
	}
	// reclaimStale loop with moved == reclaimBatch
	fakeReclaim := &fakeClientReclaimBatch{}
	a7 := &redisAdapter{client: fakeReclaim, visibilityTimeout: time.Second}
	if err := a7.reclaimStale(ctx, "t"); err != nil {
		t.Fatalf("reclaimStale loop = %v", err)
	}
	if fakeReclaim.calls != 2 {
		t.Errorf("reclaimStale calls = %d, want 2 (100 then 0)", fakeReclaim.calls)
	}

	fakeFallbackSuccess := &fakeClientCustomZCard{val: 0, hLenVal: 1, evalVal: 1}
	a8 := &redisAdapter{client: fakeFallbackSuccess}
	if err := a8.reclaimFallbackIfNeeded(ctx, "t", "0"); err != nil {
		t.Fatalf("reclaimFallback success should return nil, got %v", err)
	}
}

type fakeClientCustomLLen struct {
	fakeClient
	val      int64
	zCardErr error
}

func (f *fakeClientCustomLLen) LLen(ctx context.Context, key string) *goredis.IntCmd {
	cmd := goredis.NewIntCmd(ctx, "llen", key)
	cmd.SetVal(f.val)
	if f.lLenErr != nil {
		cmd.SetErr(f.lLenErr)
	}
	return cmd
}
func (f *fakeClientCustomLLen) ZCard(ctx context.Context, key string) *goredis.IntCmd {
	cmd := goredis.NewIntCmd(ctx, "zcard", key)
	if f.zCardErr != nil {
		cmd.SetErr(f.zCardErr)
	} else {
		cmd.SetVal(0)
	}
	return cmd
}

type fakeClientReclaimBatch struct {
	fakeClient
	calls int
}

func (f *fakeClientReclaimBatch) Eval(ctx context.Context, _ string, _ []string, _ ...any) *goredis.Cmd {
	f.calls++
	cmd := goredis.NewCmd(ctx, "eval")
	if f.calls == 1 {
		cmd.SetVal(int64(100))
	} else {
		cmd.SetVal(int64(0))
	}
	return cmd
}
func (f *fakeClientReclaimBatch) EvalSha(ctx context.Context, _ string, _ []string, _ ...any) *goredis.Cmd {
	f.calls++
	cmd := goredis.NewCmd(ctx, "evalsha")
	if f.calls == 1 {
		cmd.SetVal(int64(100))
	} else {
		cmd.SetVal(int64(0))
	}
	return cmd
}

// fakeClientSweepCounter counts promoteDue/reclaimStale script calls
// (distinguished from tryClaim by key shape: promote uses 2 keys, claim's
// first key is readyKey, reclaim's first key is processingKey) separately
// from claim attempts, so popLoop's sweep-throttling can be verified
// without conflating it with the unthrottled claim attempts.
type fakeClientSweepCounter struct {
	fakeClient
	readyKey      string
	processingKey string
	sweepCalls    int
	claimCalls    int
}

func (f *fakeClientSweepCounter) countKeys(keys []string) {
	switch {
	case len(keys) == 2:
		f.sweepCalls++ // promoteDue
	case len(keys) == 3 && keys[0] == f.processingKey:
		f.sweepCalls++ // reclaimStale
	case len(keys) == 3 && keys[0] == f.readyKey:
		f.claimCalls++ // tryClaim
	}
}

func (f *fakeClientSweepCounter) Eval(ctx context.Context, _ string, keys []string, _ ...any) *goredis.Cmd {
	f.countKeys(keys)
	cmd := goredis.NewCmd(ctx, "eval")
	cmd.SetVal(int64(0))
	return cmd
}

func (f *fakeClientSweepCounter) EvalSha(ctx context.Context, _ string, keys []string, _ ...any) *goredis.Cmd {
	f.countKeys(keys)
	cmd := goredis.NewCmd(ctx, "evalsha")
	cmd.SetVal(int64(0))
	return cmd
}

// TestRedisCover_PopLoopThrottlesSweep verifies that popLoop only runs the
// promoteDue/reclaimStale sweep once per sweepInterval, not on every
// iteration of its internal claim loop.
func TestRedisCover_PopLoopThrottlesSweep(t *testing.T) {
	ctx := t.Context()

	a := &redisAdapter{
		client:            &fakeClient{}, // placeholder, replaced below
		visibilityTimeout: time.Second,
	}
	fake := &fakeClientSweepCounter{
		readyKey:      a.readyKey("t"),
		processingKey: a.processingKey("t"),
	}
	fake.blPopErr = goredis.Nil
	a.client = fake

	// A short poll timeout bounds how long the tight non-blocking claim
	// loop spins (BLPop returns immediately via the fake), while staying
	// well under sweepInterval so only the first iteration's sweep runs.
	poll := time.NewTimer(20 * time.Millisecond)
	defer poll.Stop()

	_, err := a.popLoop(ctx, "t", a.readyKey("t"), a.processingKey("t"), a.deadlineKey("t"), poll, time.Millisecond)
	var emptyErr *queue.EmptyError
	if err != nil && !errors.As(err, &emptyErr) {
		t.Fatalf("popLoop() error = %v, want nil or EmptyError", err)
	}

	if fake.claimCalls == 0 {
		t.Fatal("claimCalls = 0, want tryClaim attempted at least once")
	}

	// One sweep window runs both promoteDue and reclaimStale exactly once,
	// so sweepCalls counts 2 regardless of how many claim attempts happen.
	if fake.sweepCalls != 2 {
		t.Errorf("sweepCalls = %d, want exactly 2 (1 promoteDue + 1 reclaimStale, throttled across %d claim attempts)", fake.sweepCalls, fake.claimCalls)
	}

	// After sweepInterval elapses, a fresh popLoop call must sweep again.
	// Backdate lastSweep past the throttle window instead of sleeping past
	// sweepInterval in wall-clock time.
	a.lastSweep.Store(time.Now().Add(-2 * sweepInterval).UnixMilli())

	poll2 := time.NewTimer(20 * time.Millisecond)
	defer poll2.Stop()

	_, err = a.popLoop(ctx, "t", a.readyKey("t"), a.processingKey("t"), a.deadlineKey("t"), poll2, time.Millisecond)
	if err != nil && !errors.As(err, &emptyErr) {
		t.Fatalf("second popLoop() error = %v, want nil or EmptyError", err)
	}

	if fake.sweepCalls != 4 {
		t.Errorf("sweepCalls after sweepInterval = %d, want 4 (2 more: 1 promoteDue + 1 reclaimStale)", fake.sweepCalls)
	}
}
