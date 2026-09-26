package static

import (
	"errors"
	"math"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/flag"
)

func TestCoverNewInvalidCoreOpts(t *testing.T) {
	t.Parallel()

	_, err := New(flag.Options{Firebase: flag.FirebaseOptions{Timeout: -time.Second}})
	if err == nil {
		t.Fatal("New(negative timeout) = nil, want core validation error")
	}
}

func TestCoverNewNotCleanPath(t *testing.T) {
	t.Parallel()

	// Literal ".." element (filepath.Join would clean it away).
	_, err := New(flag.Options{Static: flag.StaticOptions{Path: "a/../b.json"}})
	var ioerr *flag.InvalidOptionsError
	if err == nil || !errors.As(err, &ioerr) {
		t.Fatalf("New(unclean) = %v, want *InvalidOptionsError", err)
	}
}

func TestCoverNewNullJSONGivesEmptySet(t *testing.T) {
	t.Parallel()

	p := writeTempFlags(t, `null`)
	f, err := New(flag.Options{Static: flag.StaticOptions{Path: p}})
	if err != nil {
		t.Fatalf("New(null) = %v, want nil", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	if got, err := f.Bool(t.Context(), "any", true); err != nil || !got {
		t.Fatalf("Bool after null catalog = %v, %v; want true, nil", got, err)
	}
}

func TestCoverReloadStatFailsKeepsGood(t *testing.T) {
	t.Parallel()

	p := writeTempFlags(t, `{"k": true}`)
	f, err := New(flag.Options{Static: flag.StaticOptions{Path: p, Reload: true}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	if err := os.Remove(p); err != nil {
		t.Fatalf("remove: %v", err)
	}

	if got, err := f.Bool(t.Context(), "k", false); err != nil || !got {
		t.Fatalf("Bool after delete = %v, %v; want last-good true, nil", got, err)
	}
}

func TestCoverReloadReadFailsKeepsGood(t *testing.T) {
	t.Parallel()

	p := writeTempFlags(t, `{"k": true}`)
	f, err := New(flag.Options{Static: flag.StaticOptions{Path: p, Reload: true}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	if err := os.Remove(p); err != nil {
		t.Fatalf("remove: %v", err)
	}
	// A directory at the same path passes Stat but fails ReadFile.
	if err := os.Mkdir(p, 0o700); err != nil {
		t.Fatalf("mkdir over file: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(p) })

	if got, err := f.Bool(t.Context(), "k", false); err != nil || !got {
		t.Fatalf("Bool after dir-swap = %v, %v; want last-good true, nil", got, err)
	}
}

func TestCoverReloadSameContentUpdatesMod(t *testing.T) {
	t.Parallel()

	p := writeTempFlags(t, `{"k": true}`)
	f, err := New(flag.Options{Static: flag.StaticOptions{Path: p, Reload: true}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	if err := os.WriteFile(p, []byte(`{"k": true}`), 0o600); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	bumpModtime(t, p, 2*time.Second)

	if got, err := f.Bool(t.Context(), "k", false); err != nil || !got {
		t.Fatalf("Bool after same-content rewrite = %v, %v; want true, nil", got, err)
	}
}

func TestCoverReloadNullBecomesEmpty(t *testing.T) {
	t.Parallel()

	p := writeTempFlags(t, `{"k": true}`)
	f, err := New(flag.Options{Static: flag.StaticOptions{Path: p, Reload: true}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	if err := os.WriteFile(p, []byte(`null`), 0o600); err != nil {
		t.Fatalf("rewrite null: %v", err)
	}
	bumpModtime(t, p, 2*time.Second)

	if got, err := f.Bool(t.Context(), "k", false); err != nil || got {
		t.Fatalf("Bool after null reload = %v, %v; want false (missing), nil", got, err)
	}
}

func TestCoverReloadConcurrentConverges(t *testing.T) {
	t.Parallel()

	p := writeTempFlags(t, `{"k": false}`)
	f, err := New(flag.Options{Static: flag.StaticOptions{Path: p, Reload: true}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	// Hammer lookups while rewriting: concurrent reloadIfChanged calls
	// race between the pre-read hash check and the write-lock recheck,
	// exercising the lost-update guard (second hash equality branch).
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				select {
				case <-stop:
					return
				default:
					_, _ = f.Bool(t.Context(), "k", false)
				}
			}
		}()
	}

	for i := 1; i <= 4; i++ {
		content := `{"k": false}`
		if i%2 == 0 {
			content = `{"k": true}`
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatalf("rewrite: %v", err)
		}
		bumpModtime(t, p, time.Duration(2+i)*time.Second)
	}
	close(stop)
	wg.Wait()

	if got, err := f.Bool(t.Context(), "k", false); err != nil || !got {
		t.Fatalf("Bool after hammer = %v, %v; want true, nil", got, err)
	}
}

func TestCoverReloadLostUpdateGuard(t *testing.T) {
	t.Parallel()

	const key = "k"
	p := writeTempFlags(t, `{"k": true}`)
	d, err := New(flag.Options{Static: flag.StaticOptions{Path: p, Reload: true}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	dd, ok := d.(*driver)
	if !ok {
		t.Fatalf("New returned %T, want *driver", d)
	}

	const workers = 64
	const subRounds = 8

	start := make(chan struct{})
	stop := make(chan struct{})
	var rwg sync.WaitGroup
	for i := 0; i < workers; i++ {
		rwg.Add(1)
		go func() {
			defer rwg.Done()
			<-start
			for {
				select {
				case <-stop:
					return
				default:
					_, _ = dd.lookup(key)
				}
			}
		}()
	}

	var wwg sync.WaitGroup
	wwg.Add(1)
	go func() {
		defer wwg.Done()
		close(start)
		// Each modtime bump releases a synchronized rush of reloadIfChanged
		// calls that all snapshot the same pre-write lastHash under the read
		// lock (:134-137), then compute the new hash and contend for the write
		// lock (:166). The first committer sets d.lastHash; the rest see
		// hash == d.lastHash at :169 and take the lost-update guard branch.
		bump := 2
		for r := 0; r < subRounds; r++ {
			content := `{"k": false}`
			if r%2 == 0 {
				content = `{"k": true}`
			}
			if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
				t.Errorf("rewrite %d: %v", r, err)
				return
			}
			bumpModtime(t, p, time.Duration(bump)*time.Second)
			bump++
			// Yield so lookup rushes interleave without a fixed sleep.
			for range 100 {
				runtime.Gosched()
			}
		}
	}()
	wwg.Wait()

	close(stop)
	rwg.Wait()

	// r = subRounds-1 is odd, so the last written content is {"k": false}.
	got, _ := dd.lookup(key)
	if b, _ := got.(bool); b != false {
		t.Fatalf("final lookup = %v; want false (last written content)", got)
	}
}

func TestCoverRateLimitedLogSuppressed(t *testing.T) {
	t.Parallel()

	d := &driver{flags: map[string]any{}}
	d.rateLimitedLog("first %d", 1)
	d.rateLimitedLog("second %d", 2) // within interval: suppressed, no output assertable
}

func TestCoverBoolNonScalarMismatch(t *testing.T) {
	t.Parallel()

	d := &driver{flags: map[string]any{"b": []any{float64(1)}}}
	if _, err := d.Bool(t.Context(), "b", true); err == nil {
		t.Fatal("Bool(slice) = nil error, want mismatch")
	}
}

func TestCoverStringIntCase(t *testing.T) {
	t.Parallel()

	d := &driver{flags: map[string]any{"s": 42}}
	got, err := d.String(t.Context(), "s", "fb")
	if err != nil || got != "42" {
		t.Fatalf("String(int 42) = %q, %v; want %q, nil", got, err, "42")
	}
}

func TestCoverIntIntCase(t *testing.T) {
	t.Parallel()

	d := &driver{flags: map[string]any{"i": 7}}
	got, err := d.Int(t.Context(), "i", 9)
	if err != nil || got != 7 {
		t.Fatalf("Int(int 7) = %d, %v; want 7, nil", got, err)
	}
}

func TestCoverIntBoolMismatch(t *testing.T) {
	t.Parallel()

	d := &driver{flags: map[string]any{"i": true}}
	if _, err := d.Int(t.Context(), "i", 9); err == nil {
		t.Fatal("Int(bool) = nil error, want mismatch")
	}
}

func TestCoverJSONValidateKey(t *testing.T) {
	t.Parallel()

	d := &driver{flags: map[string]any{}}
	var out map[string]any
	if err := d.JSON(t.Context(), "", &out, nil); err == nil {
		t.Fatal("JSON(empty key) = nil error, want validation error")
	}
}

func TestCoverJSONFallbackMarshalFail(t *testing.T) {
	t.Parallel()

	d := &driver{flags: map[string]any{}}
	var out map[string]any
	if err := d.JSON(t.Context(), "missing", &out, func() {}); err == nil {
		t.Fatal("JSON(unmarshalable fallback) = nil error, want marshal error")
	}
}

func TestCoverJSONFallbackUnmarshalFail(t *testing.T) {
	t.Parallel()

	d := &driver{flags: map[string]any{}}
	var out map[string]any
	if err := d.JSON(t.Context(), "missing", &out, "xx"); err == nil {
		t.Fatal("JSON(string fallback into map) = nil error, want unmarshal error")
	}
}

func TestCoverJSONMarshalValueFail(t *testing.T) {
	t.Parallel()

	d := &driver{flags: map[string]any{"k": func() {}}}
	var out map[string]any
	if err := d.JSON(t.Context(), "k", &out, nil); err == nil {
		t.Fatal("JSON(func value) = nil error, want marshal error")
	}
}

func TestCoverJSONRawBadString(t *testing.T) {
	t.Parallel()

	d := &driver{flags: map[string]any{"k": "{bad"}}
	var out map[string]any
	if err := d.JSON(t.Context(), "k", &out, nil); err == nil {
		t.Fatal("JSON(bad raw string) = nil error, want unmarshal error")
	}
}

func TestCoverNewBadJSON(t *testing.T) {
	t.Parallel()

	p := writeTempFlags(t, `{bad json`)
	if _, err := New(flag.Options{Static: flag.StaticOptions{Path: p}}); err == nil {
		t.Fatal("New(bad JSON) = nil, want load error")
	}
}

func TestCoverNewUnreadableFile(t *testing.T) {
	t.Parallel()

	p := writeTempFlags(t, `{"k": true}`)
	// Stat succeeds (metadata readable) but ReadFile fails: a missing
	// read bit on an otherwise visible file.
	if err := os.Chmod(p, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(p, 0o600) })

	if _, err := New(flag.Options{Static: flag.StaticOptions{Path: p}}); err == nil {
		t.Fatal("New(unreadable) = nil, want load error")
	}
}

func TestCoverFloatToIntNaNInf(t *testing.T) {
	t.Parallel()

	if _, err := floatToInt("k", 3, math.NaN()); err == nil {
		t.Fatal("floatToInt(NaN) = nil error, want not-an-int")
	}
	if _, err := floatToInt("k", 3, math.Inf(1)); err == nil {
		t.Fatal("floatToInt(+Inf) = nil error, want not-an-int")
	}
}

func TestCoverFloatToIntOverflow(t *testing.T) {
	t.Parallel()

	got, err := floatToInt("k", 9, 1e300)
	if err == nil || got != 9 {
		t.Fatalf("floatToInt(1e300) = %d, %v; want 9, overflows error", got, err)
	}
}

func TestCoverFloatToIntPrecision(t *testing.T) {
	t.Parallel()

	// 2^53+1 is not exactly representable: must reject, not truncate.
	got, err := floatToInt("k", 9, 9007199254740993.0)
	if err == nil || got != 9 {
		t.Fatalf("floatToInt(2^53+1) = %d, %v; want 9, not-an-int error", got, err)
	}
}
