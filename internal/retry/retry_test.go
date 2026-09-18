package retry

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestPolicyNextDelayExponentialGrowth(t *testing.T) {
	p := Policy{BaseDelay: 100 * time.Millisecond, Multiplier: 2, MaxDelay: time.Hour}

	want := []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		400 * time.Millisecond,
		800 * time.Millisecond,
	}

	for i, w := range want {
		attempt := i + 1
		got := p.NextDelay(attempt)
		if got != w {
			t.Errorf("NextDelay(%d) = %v, want %v", attempt, got, w)
		}
	}
}

func TestPolicyNextDelayCapsAtMaxDelay(t *testing.T) {
	p := Policy{BaseDelay: time.Second, Multiplier: 2, MaxDelay: 5 * time.Second}

	got := p.NextDelay(10)
	if got != 5*time.Second {
		t.Errorf("NextDelay(10) = %v, want capped %v", got, 5*time.Second)
	}
}

func TestPolicyNextDelayAttemptBelowOne(t *testing.T) {
	p := Policy{BaseDelay: time.Second, Multiplier: 2}

	got0 := p.NextDelay(0)
	got1 := p.NextDelay(1)

	if got0 != got1 {
		t.Errorf("NextDelay(0) = %v, want same as NextDelay(1) = %v", got0, got1)
	}
}

func TestPolicyNextDelayMultiplierLessThanOneDisablesGrowth(t *testing.T) {
	p := Policy{BaseDelay: time.Second, Multiplier: 0.5, MaxDelay: time.Minute}

	for attempt := 1; attempt <= 5; attempt++ {
		got := p.NextDelay(attempt)
		if got != time.Second {
			t.Errorf("NextDelay(%d) = %v, want %v (no growth)", attempt, got, time.Second)
		}
	}
}

func TestPolicyNextDelayJitterWithinBounds(t *testing.T) {
	p := Policy{BaseDelay: time.Second, Multiplier: 1, Jitter: 0.2}

	const (
		iterations = 500
		lowerBound = 800 * time.Millisecond
		upperBound = 1200 * time.Millisecond
	)

	sawBelowBase := false
	sawAboveBase := false

	for i := 0; i < iterations; i++ {
		got := p.NextDelay(1)
		if got < lowerBound || got > upperBound {
			t.Fatalf("NextDelay with Jitter=0.2 = %v, want within [%v, %v]", got, lowerBound, upperBound)
		}

		if got < time.Second {
			sawBelowBase = true
		}

		if got > time.Second {
			sawAboveBase = true
		}
	}

	if !sawBelowBase || !sawAboveBase {
		t.Errorf("expected jitter to produce values on both sides of base delay across %d draws, sawBelowBase=%v sawAboveBase=%v", iterations, sawBelowBase, sawAboveBase)
	}
}

func TestPolicyNextDelayZeroJitterIsDeterministic(t *testing.T) {
	p := Policy{BaseDelay: time.Second, Multiplier: 1}

	for i := 0; i < 20; i++ {
		if got := p.NextDelay(1); got != time.Second {
			t.Fatalf("NextDelay with zero jitter = %v, want exactly %v", got, time.Second)
		}
	}
}

func TestPolicyNextDelayLinearGrowth(t *testing.T) {
	p := Policy{BaseDelay: 10 * time.Millisecond, Linear: true, MaxDelay: time.Hour}

	want := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
		40 * time.Millisecond,
	}

	for i, w := range want {
		attempt := i + 1
		got := p.NextDelay(attempt)
		if got != w {
			t.Errorf("NextDelay(%d) = %v, want %v", attempt, got, w)
		}
	}
}

func TestPolicyNextDelayLinearCapsAtMaxDelay(t *testing.T) {
	p := Policy{BaseDelay: time.Second, Linear: true, MaxDelay: 5 * time.Second}

	got := p.NextDelay(10)
	if got != 5*time.Second {
		t.Errorf("NextDelay(10) = %v, want capped %v", got, 5*time.Second)
	}
}

func TestPolicyNextDelayLinearIgnoresMultiplier(t *testing.T) {
	p := Policy{BaseDelay: time.Second, Linear: true, Multiplier: 10}

	got := p.NextDelay(2)
	if got != 2*time.Second {
		t.Errorf("NextDelay(2) = %v, want 2s (Multiplier ignored under Linear)", got)
	}
}

func TestDoSucceedsOnFirstTry(t *testing.T) {
	calls := 0

	err := Do(context.Background(), Policy{MaxAttempts: 3, BaseDelay: time.Millisecond}, func(_ context.Context) error {
		calls++
		return nil
	})

	if err != nil {
		t.Fatalf("Do() error = %v, want nil", err)
	}

	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestDoRetriesAndEventuallySucceeds(t *testing.T) {
	calls := 0

	err := Do(context.Background(), Policy{MaxAttempts: 5, BaseDelay: time.Millisecond}, func(_ context.Context) error {
		calls++
		if calls < 3 {
			return errors.New("transient")
		}

		return nil
	})

	if err != nil {
		t.Fatalf("Do() error = %v, want nil", err)
	}

	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

func TestDoExhaustsMaxAttempts(t *testing.T) {
	wantErr := errors.New("permanent")
	calls := 0

	err := Do(context.Background(), Policy{MaxAttempts: 4, BaseDelay: time.Millisecond}, func(_ context.Context) error {
		calls++
		return wantErr
	})

	if err == nil {
		t.Fatal("Do() error = nil, want non-nil")
	}

	if !errors.Is(err, wantErr) {
		t.Errorf("Do() error = %v, want wrapping %v", err, wantErr)
	}

	if calls != 4 {
		t.Errorf("calls = %d, want 4", calls)
	}
}

func TestDoRespectsContextCancellationMidRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	calls := 0

	done := make(chan error, 1)
	go func() {
		done <- Do(ctx, Policy{MaxAttempts: 100, BaseDelay: time.Hour}, func(_ context.Context) error {
			calls++
			if calls == 1 {
				cancel()
			}

			return errors.New("keep failing")
		})
	}()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Do() error = %v, want wrapping context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Do() did not return promptly after context cancellation")
	}

	if calls != 1 {
		t.Errorf("calls = %d, want 1 (no further attempt after cancellation)", calls)
	}
}

func TestDoReturnsCtxErrIfAlreadyCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	calls := 0

	err := Do(ctx, Policy{MaxAttempts: 3, BaseDelay: time.Millisecond}, func(_ context.Context) error {
		calls++
		return nil
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Do() error = %v, want context.Canceled", err)
	}

	if calls != 0 {
		t.Errorf("calls = %d, want 0", calls)
	}
}

func TestPolicyNextDelayExponentialUnaffectedByNewFields(t *testing.T) {
	// A Policy using only the pre-existing fields must behave identically
	// regardless of the new fields' zero values (Linear=false,
	// JitterMode=JitterSymmetric, JitterMax=0).
	p := Policy{BaseDelay: 100 * time.Millisecond, Multiplier: 2, MaxDelay: time.Hour}

	want := []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		400 * time.Millisecond,
		800 * time.Millisecond,
	}

	for i, w := range want {
		attempt := i + 1
		got := p.NextDelay(attempt)
		if got != w {
			t.Errorf("NextDelay(%d) = %v, want %v", attempt, got, w)
		}
	}
}

func TestPolicyNextDelayJitterAdditiveWithinBounds(t *testing.T) {
	p := Policy{BaseDelay: time.Second, Multiplier: 1, Jitter: 0.25, JitterMode: JitterAdditive}

	const (
		iterations = 500
		lowerBound = time.Second
		upperBound = 1250 * time.Millisecond
	)

	sawAboveBase := false

	for i := 0; i < iterations; i++ {
		got := p.NextDelay(1)
		if got < lowerBound || got > upperBound {
			t.Fatalf("NextDelay with JitterAdditive = %v, want within [%v, %v]", got, lowerBound, upperBound)
		}

		if got > time.Second {
			sawAboveBase = true
		}
	}

	if !sawAboveBase {
		t.Error("expected additive jitter to produce values above base delay across many draws")
	}
}

func TestPolicyNextDelayJitterFlatWithinBounds(t *testing.T) {
	p := Policy{BaseDelay: time.Second, Multiplier: 1, JitterMode: JitterFlat, JitterMax: 250 * time.Millisecond}

	const (
		iterations = 500
		lowerBound = time.Second
		upperBound = time.Second + 250*time.Millisecond
	)

	sawAboveBase := false

	for i := 0; i < iterations; i++ {
		got := p.NextDelay(1)
		if got < lowerBound || got > upperBound {
			t.Fatalf("NextDelay with JitterFlat = %v, want within [%v, %v]", got, lowerBound, upperBound)
		}

		if got > time.Second {
			sawAboveBase = true
		}
	}

	if !sawAboveBase {
		t.Error("expected flat jitter to produce values above base delay across many draws")
	}
}

func TestPolicyNextDelayJitterFlatIndependentOfDelayMagnitude(t *testing.T) {
	// A large base delay must not change the flat jitter's bound: the
	// result must never exceed base+JitterMax regardless of how large base
	// is, unlike JitterSymmetric/JitterAdditive whose spans scale with it.
	p := Policy{BaseDelay: time.Hour, Multiplier: 1, JitterMode: JitterFlat, JitterMax: 250 * time.Millisecond}

	upperBound := time.Hour + 250*time.Millisecond

	for i := 0; i < 200; i++ {
		got := p.NextDelay(1)
		if got < time.Hour || got > upperBound {
			t.Fatalf("NextDelay with JitterFlat on large base = %v, want within [%v, %v]", got, time.Hour, upperBound)
		}
	}
}

func TestDoOnRetryFiresBetweenFailuresOnly(t *testing.T) {
	type call struct {
		attempt int
		err     error
	}

	var got []call

	calls := 0

	err := Do(context.Background(), Policy{
		MaxAttempts: 5,
		BaseDelay:   time.Millisecond,
		OnRetry: func(attempt int, err error) {
			got = append(got, call{attempt, err})
		},
	}, func(_ context.Context) error {
		calls++
		if calls < 3 {
			return fmt.Errorf("transient %d", calls)
		}

		return nil
	})

	if err != nil {
		t.Fatalf("Do() error = %v, want nil", err)
	}

	if calls != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}

	want := []call{
		{1, errors.New("transient 1")},
		{2, errors.New("transient 2")},
	}

	if len(got) != len(want) {
		t.Fatalf("OnRetry called %d times, want %d: %+v", len(got), len(want), got)
	}

	for i, w := range want {
		if got[i].attempt != w.attempt || got[i].err.Error() != w.err.Error() {
			t.Errorf("OnRetry call %d = %+v, want %+v", i, got[i], w)
		}
	}
}

func TestDoOnRetryNotCalledOnImmediateSuccess(t *testing.T) {
	calls := 0

	err := Do(context.Background(), Policy{
		MaxAttempts: 3,
		BaseDelay:   time.Millisecond,
		OnRetry: func(_ int, _ error) {
			calls++
		},
	}, func(_ context.Context) error {
		return nil
	})

	if err != nil {
		t.Fatalf("Do() error = %v, want nil", err)
	}

	if calls != 0 {
		t.Errorf("OnRetry called %d times, want 0 on immediate success", calls)
	}
}

func TestDoOnRetryNotCalledAfterFinalAttempt(t *testing.T) {
	onRetryCalls := 0
	fnCalls := 0

	wantErr := errors.New("permanent")

	err := Do(context.Background(), Policy{
		MaxAttempts: 4,
		BaseDelay:   time.Millisecond,
		OnRetry: func(_ int, _ error) {
			onRetryCalls++
		},
	}, func(_ context.Context) error {
		fnCalls++
		return wantErr
	})

	if err == nil {
		t.Fatal("Do() error = nil, want non-nil")
	}

	if fnCalls != 4 {
		t.Fatalf("fnCalls = %d, want 4", fnCalls)
	}

	// OnRetry fires before each of the 3 inter-attempt sleeps (between
	// attempts 1-2, 2-3, 3-4), not after the 4th and final failed attempt.
	if onRetryCalls != 3 {
		t.Errorf("OnRetry called %d times, want 3", onRetryCalls)
	}
}

func TestDoMaxAttemptsBelowOneMeansOne(t *testing.T) {
	calls := 0

	err := Do(context.Background(), Policy{MaxAttempts: 0}, func(_ context.Context) error {
		calls++
		return errors.New("fail")
	})

	if err == nil {
		t.Fatal("Do() error = nil, want non-nil")
	}

	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}
