package redisclient

import (
	"context"
	"errors"
	"sync"
	"testing"

	goredis "github.com/redis/go-redis/v9"
)

// mustPingClosed asserts client rejects commands after its pool closed.
// go-redis checks the closed flag before dialing, so this needs no server.
func mustPingClosed(t *testing.T, client *goredis.Client) {
	t.Helper()

	if err := client.Ping(context.Background()).Err(); !errors.Is(err, goredis.ErrClosed) {
		t.Errorf("Ping() after release = %v, want ErrClosed", err)
	}
}

func TestShared_sameKey_returnsSameClient(t *testing.T) {
	t.Parallel()

	opts := Options{Addr: "127.0.0.1:1"}

	first, releaseFirst, err := Shared(opts)
	if err != nil {
		t.Fatalf("Shared() error = %v", err)
	}

	second, releaseSecond, err := Shared(opts)
	if err != nil {
		t.Fatalf("Shared() second error = %v", err)
	}

	if first != second {
		t.Fatal("Shared() with same key returned different clients")
	}

	// First release must not evict a client another borrower still holds:
	// a third call sees the same entry.
	if relErr := releaseFirst(); relErr != nil {
		t.Fatalf("releaseFirst() error = %v", relErr)
	}

	third, releaseThird, err := Shared(opts)
	if err != nil {
		t.Fatalf("Shared() third error = %v", err)
	}

	if third != first {
		t.Fatal("Shared() after one release returned a new client, want the shared one")
	}

	if err := releaseThird(); err != nil {
		t.Fatalf("releaseThird() error = %v", err)
	}

	if err := releaseSecond(); err != nil {
		t.Fatalf("releaseSecond() error = %v", err)
	}

	mustPingClosed(t, first)
}

func TestShared_releaseClosesAtZero(t *testing.T) {
	t.Parallel()

	opts := Options{Addr: "127.0.0.1:2"}

	client, release, err := Shared(opts)
	if err != nil {
		t.Fatalf("Shared() error = %v", err)
	}

	if relErr := release(); relErr != nil {
		t.Fatalf("release() error = %v", relErr)
	}

	mustPingClosed(t, client)

	// The entry is evicted, so a later call builds a fresh client.
	again, releaseAgain, err := Shared(opts)
	if err != nil {
		t.Fatalf("Shared() again error = %v", err)
	}
	t.Cleanup(func() { _ = releaseAgain() })

	if again == client {
		t.Error("Shared() after zero refs returned the closed client, want a fresh one")
	}
}

func TestShared_releaseIsIdempotent(t *testing.T) {
	t.Parallel()

	opts := Options{Addr: "127.0.0.1:3"}

	_, release, err := Shared(opts)
	if err != nil {
		t.Fatalf("Shared() error = %v", err)
	}

	if err := release(); err != nil {
		t.Fatalf("first release() error = %v", err)
	}

	if err := release(); err != nil {
		t.Errorf("second release() error = %v, want nil (idempotent)", err)
	}
}

func TestShared_differentKey_returnsDifferentClients(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a    Options
		b    Options
	}{
		{name: "addr", a: Options{Addr: "127.0.0.1:10"}, b: Options{Addr: "127.0.0.1:11"}},
		{name: "db", a: Options{Addr: "127.0.0.1:12", DB: 0}, b: Options{Addr: "127.0.0.1:12", DB: 1}},
		{name: "password", a: Options{Addr: "127.0.0.1:13", Password: "a"}, b: Options{Addr: "127.0.0.1:13", Password: "b"}},
		{name: "tls", a: Options{Addr: "127.0.0.1:14"}, b: Options{Addr: "127.0.0.1:14", TLS: true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			first, releaseFirst, err := Shared(tt.a)
			if err != nil {
				t.Fatalf("Shared(a) error = %v", err)
			}
			t.Cleanup(func() { _ = releaseFirst() })

			second, releaseSecond, err := Shared(tt.b)
			if err != nil {
				t.Fatalf("Shared(b) error = %v", err)
			}
			t.Cleanup(func() { _ = releaseSecond() })

			if first == second {
				t.Errorf("Shared() with different %s returned the same client", tt.name)
			}
		})
	}
}

func TestShared_firstCreatorPoolOptionsWin(t *testing.T) {
	t.Parallel()

	opts := Options{Addr: "127.0.0.1:4", PoolSize: 5}

	first, releaseFirst, err := Shared(opts)
	if err != nil {
		t.Fatalf("Shared() error = %v", err)
	}
	t.Cleanup(func() { _ = releaseFirst() })

	second, releaseSecond, err := Shared(Options{Addr: "127.0.0.1:4", PoolSize: 99})
	if err != nil {
		t.Fatalf("Shared() second error = %v", err)
	}
	t.Cleanup(func() { _ = releaseSecond() })

	if first != second {
		t.Fatal("Shared() differing only in pool size returned different clients")
	}

	if got := first.Options().PoolSize; got != 5 {
		t.Errorf("shared client PoolSize = %d, want first creator's 5", got)
	}
}

func TestShared_failedCreateIsRetried(t *testing.T) {
	t.Parallel()

	r := newSharedRegistry()

	calls := 0
	sentinel := errors.New("boom")
	r.open = func(o *goredis.Options) (*goredis.Client, error) {
		calls++
		if calls == 1 {
			return nil, sentinel
		}

		return goredis.NewClient(o), nil
	}

	key := "redis:test"
	opts := &goredis.Options{Addr: "127.0.0.1:5"}

	if _, _, err := r.acquire(key, opts); !errors.Is(err, sentinel) {
		t.Fatalf("acquire() error = %v, want sentinel", err)
	}

	client, release, err := r.acquire(key, opts)
	if err != nil {
		t.Fatalf("acquire() retry error = %v", err)
	}
	t.Cleanup(func() { _ = release() })

	if client == nil {
		t.Fatal("acquire() retry returned nil client")
	}

	if calls != 2 {
		t.Errorf("open calls = %d, want 2 (evict on failure, retry)", calls)
	}
}

func TestShared_concurrentFirstCreate_returnsSameClient(t *testing.T) {
	t.Parallel()

	opts := Options{Addr: "127.0.0.1:6"}

	const workers = 8

	clients := make([]*goredis.Client, workers)
	releases := make([]func() error, workers)

	var wg sync.WaitGroup

	for i := range workers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			client, release, err := Shared(opts)
			if err != nil {
				t.Errorf("Shared() error = %v", err)

				return
			}

			clients[i] = client
			releases[i] = release
		}()
	}

	wg.Wait()

	for i := 1; i < workers; i++ {
		if clients[i] != clients[0] {
			t.Fatalf("Shared() worker %d got a different client", i)
		}
	}

	for _, release := range releases {
		if release == nil {
			continue
		}

		if err := release(); err != nil {
			t.Errorf("release() error = %v", err)
		}
	}

	mustPingClosed(t, clients[0])
}

func TestSharedKey_excludesPoolSizing(t *testing.T) {
	t.Parallel()

	base := &goredis.Options{Addr: "127.0.0.1:7", DB: 3, Password: "pw", PoolSize: 10, MinIdleConns: 4}

	tuned := *base
	tuned.PoolSize = 500
	tuned.MinIdleConns = 50

	if got, want := sharedKey(&tuned), sharedKey(base); got != want {
		t.Errorf("sharedKey() = %q, want %q (pool sizing must not affect the key)", got, want)
	}
}

func TestSharedKey_includesIdentity(t *testing.T) {
	t.Parallel()

	base := &goredis.Options{Addr: "127.0.0.1:8", DB: 1, Password: "pw"}

	diffDB := *base
	diffDB.DB = 2

	diffPW := *base
	diffPW.Password = "other"

	diffAddr := *base
	diffAddr.Addr = "127.0.0.1:9"

	for name, other := range map[string]*goredis.Options{
		"db":       &diffDB,
		"password": &diffPW,
		"addr":     &diffAddr,
	} {
		if sharedKey(other) == sharedKey(base) {
			t.Errorf("sharedKey() ignores %s identity", name)
		}
	}
}

func BenchmarkSharedAcquireRelease(b *testing.B) {
	opts := Options{Addr: "127.0.0.1:20"}

	// Seed the registry entry with idle-conn warming disabled so the held
	// client does not dial in the background; Shared still pays its full
	// per-call cost (toRedisOptions + key + refcount).
	warm, err := toRedisOptions(opts)
	if err != nil {
		b.Fatalf("toRedisOptions() error = %v", err)
	}

	warm.MinIdleConns = -1

	if _, hold, err := shared.acquire(sharedKey(warm), warm); err != nil {
		b.Fatalf("seed acquire() error = %v", err)
	} else {
		defer func() { _ = hold() }()
	}

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		_, release, err := Shared(opts)
		if err != nil {
			b.Fatalf("Shared() error = %v", err)
		}

		if err := release(); err != nil {
			b.Fatalf("release() error = %v", err)
		}
	}
}

// BenchmarkPrivateClientBuild measures the per-battery client+pool
// construction the shared registry avoids on every borrower after the first.
// Idle-conn warming is disabled so the benchmark stays offline (New would
// eagerly dial MinIdleConns, which the shared path only pays once).
func BenchmarkPrivateClientBuild(b *testing.B) {
	opts := Options{Addr: "127.0.0.1:21"}

	redisOpt, err := toRedisOptions(opts)
	if err != nil {
		b.Fatalf("toRedisOptions() error = %v", err)
	}

	redisOpt.MinIdleConns = -1

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		_ = goredis.NewClient(redisOpt).Close()
	}
}
