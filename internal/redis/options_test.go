package redis

import (
	"crypto/tls"
	"errors"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

func assertRedisOptionsEqual(t *testing.T, got, want *goredis.Options) {
	t.Helper()

	if got.Addr != want.Addr {
		t.Errorf("Addr = %q, want %q", got.Addr, want.Addr)
	}

	if got.Password != want.Password {
		t.Errorf("Password = %q, want %q", got.Password, want.Password)
	}

	if got.DB != want.DB {
		t.Errorf("DB = %d, want %d", got.DB, want.DB)
	}

	if got.PoolSize != want.PoolSize {
		t.Errorf("PoolSize = %d, want %d", got.PoolSize, want.PoolSize)
	}

	if got.MinIdleConns != want.MinIdleConns {
		t.Errorf("MinIdleConns = %d, want %d", got.MinIdleConns, want.MinIdleConns)
	}

	if got.PoolTimeout != want.PoolTimeout {
		t.Errorf("PoolTimeout = %v, want %v", got.PoolTimeout, want.PoolTimeout)
	}

	if got.ConnMaxIdleTime != want.ConnMaxIdleTime {
		t.Errorf("ConnMaxIdleTime = %v, want %v", got.ConnMaxIdleTime, want.ConnMaxIdleTime)
	}

	if got.ConnMaxLifetime != want.ConnMaxLifetime {
		t.Errorf("ConnMaxLifetime = %v, want %v", got.ConnMaxLifetime, want.ConnMaxLifetime)
	}

	if (got.TLSConfig == nil) != (want.TLSConfig == nil) {
		t.Fatalf("TLSConfig nil = %v, want nil = %v", got.TLSConfig == nil, want.TLSConfig == nil)
	}

	if want.TLSConfig != nil && got.TLSConfig.MinVersion != want.TLSConfig.MinVersion {
		t.Errorf("TLSConfig.MinVersion = %v, want %v", got.TLSConfig.MinVersion, want.TLSConfig.MinVersion)
	}
}

func TestOptions_toRedisOptions_plainAddr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   Options
		want goredis.Options
	}{
		{
			name: "empty defaults",
			in:   Options{},
			want: goredis.Options{Addr: "localhost:6379", MinIdleConns: 2},
		},
		{
			name: "whitespace addr and password trimmed",
			in:   Options{Addr: "  10.0.0.1:6380 ", Password: " secret "},
			want: goredis.Options{Addr: "10.0.0.1:6380", Password: "secret", MinIdleConns: 2},
		},
		{
			name: "explicit minidle kept",
			in:   Options{MinIdleConns: 5},
			want: goredis.Options{Addr: "localhost:6379", MinIdleConns: 5},
		},
		{
			name: "pool tuning passthrough",
			in:   Options{DB: 2, PoolSize: 20, MinIdleConns: 4, PoolTimeout: time.Second, MaxConnIdleTime: time.Minute, MaxConnLifetime: time.Hour},
			want: goredis.Options{Addr: "localhost:6379", DB: 2, PoolSize: 20, MinIdleConns: 4, PoolTimeout: time.Second, ConnMaxIdleTime: time.Minute, ConnMaxLifetime: time.Hour},
		},
		{
			name: "tls sets version floor",
			in:   Options{TLS: true},
			want: goredis.Options{Addr: "localhost:6379", MinIdleConns: 2, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := tt.in.toRedisOptions()
			if err != nil {
				t.Fatalf("toRedisOptions() error = %v", err)
			}

			assertRedisOptionsEqual(t, got, &tt.want)
		})
	}
}

func TestOptions_toRedisOptions_urlForms(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		in       Options
		wantAddr string
		wantPass string
		wantDB   int
		wantTLS  bool
	}{
		{name: "redis scheme", in: Options{Addr: "redis://localhost:6379"}, wantAddr: "localhost:6379"},
		{name: "rediss implies tls", in: Options{Addr: "rediss://localhost:6379"}, wantAddr: "localhost:6379", wantTLS: true},
		{name: "uppercase scheme honored", in: Options{Addr: "REDIS://h:6379"}, wantAddr: "h:6379"},
		{name: "uppercase rediss implies tls", in: Options{Addr: "REDISS://h:6379"}, wantAddr: "h:6379", wantTLS: true},
		{name: "tls flag with plain addr", in: Options{Addr: "plain:6379", TLS: true}, wantAddr: "plain:6379", wantTLS: true},
		{name: "tls flag with redis url", in: Options{Addr: "redis://h:6379", TLS: true}, wantAddr: "h:6379", wantTLS: true},
		{name: "url userinfo password", in: Options{Addr: "redis://:s3cret@h:6379"}, wantAddr: "h:6379", wantPass: "s3cret"},
		{name: "explicit password wins over url", in: Options{Addr: "redis://:urlpw@h:6379", Password: "explicit"}, wantAddr: "h:6379", wantPass: "explicit"},
		{name: "url path db ignored, options db kept", in: Options{Addr: "redis://h:6379/3", DB: 2}, wantAddr: "h:6379", wantDB: 2},
		{name: "url minidle defaults to warm pool", in: Options{Addr: "redis://h:6379"}, wantAddr: "h:6379"},
		{name: "url explicit minidle kept", in: Options{Addr: "redis://h:6379", MinIdleConns: 7}, wantAddr: "h:6379"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := tt.in.toRedisOptions()
			if err != nil {
				t.Fatalf("toRedisOptions() error = %v", err)
			}

			if got.Addr != tt.wantAddr {
				t.Errorf("Addr = %q, want %q", got.Addr, tt.wantAddr)
			}

			if got.Password != tt.wantPass {
				t.Errorf("Password = %q, want %q", got.Password, tt.wantPass)
			}

			if got.DB != tt.wantDB {
				t.Errorf("DB = %d, want %d", got.DB, tt.wantDB)
			}

			if (got.TLSConfig != nil) != tt.wantTLS {
				t.Errorf("TLS enabled = %v, want %v", got.TLSConfig != nil, tt.wantTLS)
			}

			if tt.wantTLS && got.TLSConfig.MinVersion != tls.VersionTLS12 {
				t.Errorf("TLSConfig.MinVersion = %v, want %v", got.TLSConfig.MinVersion, tls.VersionTLS12)
			}

			wantIdle := max(tt.in.MinIdleConns, 2)
			if got.MinIdleConns != wantIdle {
				t.Errorf("MinIdleConns = %d, want %d", got.MinIdleConns, wantIdle)
			}
		})
	}
}

func TestOptions_toRedisOptions_invalidAddr_typedError(t *testing.T) {
	t.Parallel()

	t.Run("missing host", func(t *testing.T) {
		t.Parallel()

		for _, addr := range []string{"redis://", "redis:///db0", "rediss://"} {
			_, err := (Options{Addr: addr}).toRedisOptions()
			if err == nil {
				t.Fatalf("toRedisOptions(%q) = nil, want InvalidAddressError", addr)
			}

			if !errors.Is(err, ErrInvalidAddress) {
				t.Errorf("errors.Is(err, ErrInvalidAddress) = false (err = %v)", err)
			}

			var addrErr *InvalidAddressError
			if !errors.As(err, &addrErr) {
				t.Fatalf("errors.As(err, InvalidAddressError) = false (err = %T %v)", err, err)
			}

			if addrErr.Addr != addr {
				t.Errorf("InvalidAddressError.Addr = %q, want %q", addrErr.Addr, addr)
			}
		}
	})

	t.Run("unparsable url wraps parse sentinel", func(t *testing.T) {
		t.Parallel()

		_, err := (Options{Addr: "redis://[::1"}).toRedisOptions()
		if err == nil {
			t.Fatal("toRedisOptions() = nil, want parse error")
		}

		if !errors.Is(err, ErrInvalidAddress) {
			t.Errorf("errors.Is(err, ErrInvalidAddress) = false (err = %v)", err)
		}

		if !errors.Is(err, ErrParseAddress) {
			t.Errorf("errors.Is(err, ErrParseAddress) = false (err = %v)", err)
		}
	})
}

func TestOptions_toRedisOptions_requireTLS(t *testing.T) {
	t.Parallel()

	t.Run("default allows plaintext", func(t *testing.T) {
		t.Parallel()

		got, err := (Options{Addr: "localhost:6379"}).toRedisOptions()
		if err != nil {
			t.Fatalf("toRedisOptions() error = %v, want nil", err)
		}

		if got.TLSConfig != nil {
			t.Errorf("TLSConfig = %v, want nil", got.TLSConfig)
		}
	})

	t.Run("requireTLS with plaintext addr fails", func(t *testing.T) {
		t.Parallel()

		_, err := (Options{Addr: "localhost:6379", RequireTLS: true}).toRedisOptions()
		if err == nil {
			t.Fatal("toRedisOptions() = nil error, want PlaintextRejectedError")
		}

		if !errors.Is(err, ErrPlaintextRejected) {
			t.Errorf("errors.Is(err, ErrPlaintextRejected) = false (err = %v)", err)
		}

		var plaintextErr *PlaintextRejectedError
		if !errors.As(err, &plaintextErr) {
			t.Fatalf("errors.As(err, PlaintextRejectedError) = false (err = %T %v)", err, err)
		}
	})

	t.Run("requireTLS with redis url addr fails", func(t *testing.T) {
		t.Parallel()

		_, err := (Options{Addr: "redis://h:6379", RequireTLS: true}).toRedisOptions()
		if err == nil {
			t.Fatal("toRedisOptions() = nil error, want PlaintextRejectedError")
		}

		if !errors.Is(err, ErrPlaintextRejected) {
			t.Errorf("errors.Is(err, ErrPlaintextRejected) = false (err = %v)", err)
		}
	})

	t.Run("requireTLS with rediss url addr succeeds", func(t *testing.T) {
		t.Parallel()

		got, err := (Options{Addr: "rediss://h:6379", RequireTLS: true}).toRedisOptions()
		if err != nil {
			t.Fatalf("toRedisOptions() error = %v, want nil", err)
		}

		if got.TLSConfig == nil {
			t.Fatal("TLSConfig = nil, want set")
		}

		if got.TLSConfig.MinVersion != tls.VersionTLS12 {
			t.Errorf("TLSConfig.MinVersion = %v, want %v", got.TLSConfig.MinVersion, tls.VersionTLS12)
		}
	})

	t.Run("requireTLS with explicit TLS true on plain addr succeeds", func(t *testing.T) {
		t.Parallel()

		got, err := (Options{Addr: "plain:6379", TLS: true, RequireTLS: true}).toRedisOptions()
		if err != nil {
			t.Fatalf("toRedisOptions() error = %v, want nil", err)
		}

		if got.TLSConfig == nil {
			t.Fatal("TLSConfig = nil, want set")
		}
	})
}
