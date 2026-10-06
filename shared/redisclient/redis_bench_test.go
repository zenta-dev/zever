package redisclient

import "testing"

// BenchmarkToRedisOptions measures resolving Options into go-redis options
// for a redis:// URL carrying a password.
func BenchmarkToRedisOptions(b *testing.B) {
	opts := Options{Addr: "redis://:s3cret@127.0.0.1:6379", DB: 1, PoolSize: 10}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := toRedisOptions(opts); err != nil {
			b.Fatalf("toRedisOptions() error = %v", err)
		}
	}
}

// BenchmarkSharedAcquireReleaseSerial measures the refcounted registry under
// serial borrowers of one key.
func BenchmarkSharedAcquireReleaseSerial(b *testing.B) {
	opts := Options{Addr: "127.0.0.1:22"}

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

	for b.Loop() {
		_, release, err := Shared(opts)
		if err != nil {
			b.Fatalf("Shared() error = %v", err)
		}

		if err := release(); err != nil {
			b.Fatalf("release() error = %v", err)
		}
	}
}

// BenchmarkSharedAcquireReleaseParallel measures the refcounted registry under
// concurrent borrowers of one key.
func BenchmarkSharedAcquireReleaseParallel(b *testing.B) {
	opts := Options{Addr: "127.0.0.1:22"}

	// Seed the entry with idle-conn warming disabled so the held client does
	// not dial in the background; the shared path still pays its full cost.
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

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, release, err := Shared(opts)
			if err != nil {
				b.Errorf("Shared() error = %v", err)

				return
			}

			if err := release(); err != nil {
				b.Errorf("release() error = %v", err)

				return
			}
		}
	})
}
