package job

import (
	"context"
	"encoding/json/v2"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestJobCoreResetClearsState(t *testing.T) {
	Reset()

	// Arrange: populate definitions, middleware, batchCallbacks.
	if err := Register("reset-a", func(context.Context, string) error { return nil }); err != nil {
		t.Fatalf("Register reset-a: %v", err)
	}
	if err := Use(func(next Handler) Handler { return next }); err != nil {
		t.Fatalf("Use: %v", err)
	}
	batchCallbacks.Lock()
	batchCallbacks.m["batch-reset"] = &batchCallback{}
	batchCallbacks.Unlock()

	// Act.
	Reset()

	// Assert definitions cleared.
	if _, ok := Lookup("reset-a"); ok {
		t.Fatalf("Lookup after Reset: want miss")
	}
	mu.RLock()
	midLen := len(middleware)
	mu.RUnlock()
	if midLen != 0 {
		t.Fatalf("middleware after Reset: len=%d want 0", midLen)
	}
	batchCallbacks.Lock()
	bcLen := len(batchCallbacks.m)
	batchCallbacks.Unlock()
	if bcLen != 0 {
		t.Fatalf("batchCallbacks after Reset: len=%d want 0", bcLen)
	}
}

func TestJobCoreUseNil(t *testing.T) {
	Reset()
	err := Use(nil)
	if !errors.Is(err, ErrUseMiddlewareNil) {
		t.Fatalf("Use(nil) err=%v want ErrUseMiddlewareNil", err)
	}
	mu.RLock()
	midLen := len(middleware)
	mu.RUnlock()
	if midLen != 0 {
		t.Fatalf("middleware len after Use(nil)=%d want 0", midLen)
	}
}

func TestJobCoreUseAppend(t *testing.T) {
	Reset()
	m1 := func(next Handler) Handler { return next }
	if err := Use(m1); err != nil {
		t.Fatalf("Use m1: %v", err)
	}
	mu.RLock()
	midLen := len(middleware)
	mu.RUnlock()
	if midLen != 1 {
		t.Fatalf("middleware len=%d want 1", midLen)
	}
}

func TestJobCoreUseOrderPreserved(t *testing.T) {
	Reset()
	var order []int
	m1 := func(next Handler) Handler {
		return func(ctx context.Context, p Payload) error {
			order = append(order, 1)
			return next(ctx, p)
		}
	}
	m2 := func(next Handler) Handler {
		return func(ctx context.Context, p Payload) error {
			order = append(order, 2)
			return next(ctx, p)
		}
	}
	if err := Use(m1); err != nil {
		t.Fatalf("Use m1: %v", err)
	}
	if err := Use(m2); err != nil {
		t.Fatalf("Use m2: %v", err)
	}
	mu.RLock()
	if len(middleware) != 2 {
		mu.RUnlock()
		t.Fatalf("middleware len=%d want 2", len(middleware))
	}
	// Verify order by invoking stored middleware.
	mids := make([]Middleware, len(middleware))
	copy(mids, middleware)
	mu.RUnlock()

	base := func(context.Context, Payload) error { order = append(order, 99); return nil }
	// Build chain in insertion order (m1 outermost, m2 inner) similar to typical use.
	// Here test that mids[0] is m1 and mids[1] is m2 by checking tags via separate invocation.
	order = nil
	h0 := mids[0](base)
	if err := h0(context.Background(), nil); err != nil {
		t.Fatalf("mids[0] invoke: %v", err)
	}
	if len(order) != 2 || order[0] != 1 || order[1] != 99 {
		t.Fatalf("mids[0] order=%v want [1 99]", order)
	}
	order = nil
	h1 := mids[1](base)
	if err := h1(context.Background(), nil); err != nil {
		t.Fatalf("mids[1] invoke: %v", err)
	}
	if len(order) != 2 || order[0] != 2 || order[1] != 99 {
		t.Fatalf("mids[1] order=%v want [2 99]", order)
	}
}

func TestJobCoreRegisterEmptyName(t *testing.T) {
	Reset()
	err := Register("   ", func(context.Context, string) error { return nil })
	if !errors.Is(err, ErrRegisterNameEmpty) {
		t.Fatalf("Register empty err=%v want ErrRegisterNameEmpty", err)
	}
}

func TestJobCoreRegisterEmptyNameTrim(t *testing.T) {
	Reset()
	err := Register("", func(context.Context, string) error { return nil })
	if !errors.Is(err, ErrRegisterNameEmpty) {
		t.Fatalf("Register empty string err=%v want ErrRegisterNameEmpty", err)
	}
}

func TestJobCoreRegisterNilHandle(t *testing.T) {
	Reset()
	err := Register[string]("nil-handle", nil)
	if !errors.Is(err, ErrRegisterHandleNil) {
		t.Fatalf("Register nil handle err=%v want ErrRegisterHandleNil", err)
	}
}

func TestJobCoreRegisterTrimSpaceSuccess(t *testing.T) {
	Reset()
	if err := Register("  spaced  ", func(context.Context, string) error { return nil }); err != nil {
		t.Fatalf("Register spaced: %v", err)
	}
	if _, ok := Lookup("spaced"); !ok {
		t.Fatalf("Lookup spaced after trimmed register: miss")
	}
	if _, ok := Lookup("  spaced  "); ok {
		t.Fatalf("Lookup with spaces should miss, trimmed name stored")
	}
}

func TestJobCoreRegisterDuplicate(t *testing.T) {
	Reset()
	h := func(context.Context, string) error { return nil }
	if err := Register("dup-job", h); err != nil {
		t.Fatalf("first Register dup-job: %v", err)
	}
	err := Register("dup-job", h)
	if err == nil {
		t.Fatal("second Register dup-job: want error")
	}
	if !errors.Is(err, ErrDuplicateJob) {
		t.Fatalf("errors.Is dup err=%v want ErrDuplicateJob", err)
	}
	var dupErr *DuplicateJobError
	if !errors.As(err, &dupErr) {
		t.Fatalf("errors.As dup err=%T %v want *DuplicateJobError", err, err)
	}
	if dupErr.Name != "dup-job" {
		t.Fatalf("DuplicateJobError.Name=%q want %q", dupErr.Name, "dup-job")
	}
}

func TestJobCoreRegisterOptions(t *testing.T) {
	Reset()
	custom := RetryPolicy{MaxAttempts: 3, BaseDelay: 10 * time.Second}
	if err := Register("with-opts", func(context.Context, string) error { return nil }, WithRetryPolicy(custom), WithPriority(PriorityHigh)); err != nil {
		t.Fatalf("Register with-opts: %v", err)
	}
	def, ok := Lookup("with-opts")
	if !ok {
		t.Fatal("Lookup with-opts: miss")
	}
	if def.policy != custom {
		t.Fatalf("policy=%v want %v", def.policy, custom)
	}
	if def.priority != PriorityHigh {
		t.Fatalf("priority=%v want %v", def.priority, PriorityHigh)
	}
}

func TestJobCoreRegisterHandlerDecodeError(t *testing.T) {
	Reset()
	type args struct {
		X string `json:"x"`
	}
	if err := Register("decode-err", func(context.Context, args) error { return nil }); err != nil {
		t.Fatalf("Register decode-err: %v", err)
	}
	def, ok := Lookup("decode-err")
	if !ok {
		t.Fatal("Lookup decode-err: miss")
	}
	err := def.handler(context.Background(), Payload([]byte(`{invalid`)))
	if err == nil {
		t.Fatal("handler with bad JSON: want error")
	}
	if !strings.Contains(err.Error(), `"decode-err"`) {
		t.Fatalf("decode error %q does not contain job name", err.Error())
	}
	inner := json.Unmarshal([]byte(`{invalid`), &args{})
	if inner == nil {
		t.Fatal("json.Unmarshal of bad payload unexpectedly succeeded")
	}
	// The handler error should wrap something; at least unwrap once.
	if errors.Unwrap(err) == nil {
		t.Fatalf("decode error %v should wrap json error", err)
	}
}

func TestJobCoreRegisterHandlerSuccess(t *testing.T) {
	Reset()
	type args struct {
		X string `json:"x"`
	}
	var got string
	var handlerCalled bool
	if err := Register("success-job", func(_ context.Context, a args) error {
		handlerCalled = true
		got = a.X
		return nil
	}); err != nil {
		t.Fatalf("Register success-job: %v", err)
	}
	def, ok := Lookup("success-job")
	if !ok {
		t.Fatal("Lookup success-job: miss")
	}
	payload, err := json.Marshal(args{X: "hello"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := def.handler(context.Background(), Payload(payload)); err != nil {
		t.Fatalf("handler success: %v", err)
	}
	if !handlerCalled {
		t.Fatal("handler not called")
	}
	if got != "hello" {
		t.Fatalf("handler arg X=%q want %q", got, "hello")
	}
}

func TestJobCoreRegisterHandlerPropagatesError(t *testing.T) {
	Reset()
	type args struct {
		X string `json:"x"`
	}
	sentinel := errors.New("handle boom")
	if err := Register("prop-err", func(context.Context, args) error { return sentinel }); err != nil {
		t.Fatalf("Register prop-err: %v", err)
	}
	def, ok := Lookup("prop-err")
	if !ok {
		t.Fatal("Lookup prop-err: miss")
	}
	payload, err := json.Marshal(args{X: "v"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	err = def.handler(context.Background(), Payload(payload))
	if !errors.Is(err, sentinel) {
		t.Fatalf("handler err=%v want sentinel", err)
	}
}

func TestJobCoreLookupMissAndHit(t *testing.T) {
	Reset()
	if _, ok := Lookup("nope"); ok {
		t.Fatal("Lookup nope before register: want miss")
	}
	if err := Register("hit", func(context.Context, string) error { return nil }); err != nil {
		t.Fatalf("Register hit: %v", err)
	}
	def, ok := Lookup("hit")
	if !ok {
		t.Fatal("Lookup hit: want hit")
	}
	if def.handler == nil {
		t.Fatal("Lookup hit: handler nil")
	}
	if _, ok := Lookup("missing"); ok {
		t.Fatal("Lookup missing: want miss")
	}
}

func TestJobCoreOptionDirect(t *testing.T) {
	Reset()
	def := Definition{}
	p := RetryPolicy{MaxAttempts: 7, BaseDelay: 2 * time.Second}
	WithRetryPolicy(p)(&def)
	if def.policy != p {
		t.Fatalf("WithRetryPolicy policy=%v want %v", def.policy, p)
	}
	WithPriority(PriorityMedium)(&def)
	if def.priority != PriorityMedium {
		t.Fatalf("WithPriority priority=%v want %v", def.priority, PriorityMedium)
	}
	WithPriority(PriorityHigh)(&def)
	if def.priority != PriorityHigh {
		t.Fatalf("WithPriority High priority=%v want %v", def.priority, PriorityHigh)
	}
}

func TestPriorityString(t *testing.T) {
	Reset()
	cases := []struct {
		p    Priority
		want string
	}{
		{PriorityLow, "low"},
		{PriorityMedium, "medium"},
		{PriorityHigh, "high"},
		{Priority(109), "unknown"},
	}
	for _, c := range cases {
		got := c.p.String()
		if got != c.want {
			t.Errorf("Priority(%d).String()=%q want %q", uint8(c.p), got, c.want)
		}
	}
}

func TestRetryDefaultPolicy(t *testing.T) {
	Reset()
	p := DefaultRetryPolicy()
	if p.MaxAttempts != 25 {
		t.Fatalf("DefaultRetryPolicy MaxAttempts=%d want 25", p.MaxAttempts)
	}
	if p.BaseDelay != 5*time.Second {
		t.Fatalf("DefaultRetryPolicy BaseDelay=%v want 5s", p.BaseDelay)
	}
}

func TestRetryBackoff(t *testing.T) {
	Reset()
	base := 5 * time.Second
	defPolicy := RetryPolicy{BaseDelay: base}

	cases := []struct {
		name    string
		policy  RetryPolicy
		attempt int
		want    time.Duration
	}{
		{"attempt 0 returns BaseDelay", defPolicy, 0, base},
		{"attempt -5 returns BaseDelay", defPolicy, -5, base},
		{"attempt 1 returns BaseDelay", defPolicy, 1, base},
		{"attempt 2 returns 2*Base", defPolicy, 2, 2 * base},
		{"attempt 3 returns 4*Base", defPolicy, 3, 4 * base},
		{"attempt 4 returns 8*Base", defPolicy, 4, 8 * base},
		{"clamp at 24h with huge base 24h attempt 2", RetryPolicy{BaseDelay: 24 * time.Hour}, 2, 24 * time.Hour},
		{"overflow shift>20 attempt 30 with base 1ms uses 1<<20", RetryPolicy{BaseDelay: time.Millisecond}, 30, time.Millisecond * (1 << 20)},
		{"overflow attempt 30 with def base clamps to maxDelay", defPolicy, 30, 24 * time.Hour},
		{"custom base 10ms attempt 2", RetryPolicy{BaseDelay: 10 * time.Millisecond}, 2, 20 * time.Millisecond},
		{"custom base 10ms attempt 3", RetryPolicy{BaseDelay: 10 * time.Millisecond}, 3, 40 * time.Millisecond},
		{"attempt 21 still uses 1<<20", RetryPolicy{BaseDelay: time.Millisecond}, 21, time.Millisecond * (1 << 20)},
		{"attempt 20 uses 1<<19", RetryPolicy{BaseDelay: time.Millisecond}, 20, time.Millisecond * (1 << 19)},
	}

	for _, c := range cases {
		got := c.policy.Backoff(c.attempt)
		if got != c.want {
			t.Errorf("Backoff %s: Backoff(%d)=%v want %v", c.name, c.attempt, got, c.want)
		}
	}
}

func TestRetryBackoffCustomBaseClamp(t *testing.T) {
	Reset()
	// BaseDelay huge, attempt 2 should clamp to 24h.
	p := RetryPolicy{BaseDelay: 24 * time.Hour}
	if got := p.Backoff(2); got != 24*time.Hour {
		t.Fatalf("Backoff huge base attempt2=%v want 24h", got)
	}
	// BaseDelay small overflow shift capped at 20, attempt 30 should equal attempt 21.
	pSmall := RetryPolicy{BaseDelay: time.Nanosecond}
	if got30, got21 := pSmall.Backoff(30), pSmall.Backoff(21); got30 != got21 {
		t.Fatalf("Backoff small base attempt30=%v attempt21=%v want equal", got30, got21)
	}
}
