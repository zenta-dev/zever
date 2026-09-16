package argon2

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/password"
)

func TestParseOptionsTypedZero(t *testing.T) {
	t.Parallel()

	opts, err := ParseOptionsTyped(password.Options{})
	if err != nil {
		t.Fatalf("ParseOptionsTyped() = %v, want nil", err)
	}

	if opts.Time != DefaultTime {
		t.Fatalf("Time = %d, want %d", opts.Time, DefaultTime)
	}

	if opts.Memory != DefaultMemory {
		t.Fatalf("Memory = %d, want %d", opts.Memory, DefaultMemory)
	}

	if opts.Threads != DefaultThreads {
		t.Fatalf("Threads = %d, want %d", opts.Threads, DefaultThreads)
	}

	if opts.SaltLen != DefaultSaltLen {
		t.Fatalf("SaltLen = %d, want %d", opts.SaltLen, DefaultSaltLen)
	}

	if opts.KeyLen != DefaultKeyLen {
		t.Fatalf("KeyLen = %d, want %d", opts.KeyLen, DefaultKeyLen)
	}
}

func TestParseOptionsTypedPartial(t *testing.T) {
	t.Parallel()

	opts, err := ParseOptionsTyped(password.Options{Time: 1})
	if err != nil {
		t.Fatalf("ParseOptionsTyped() = %v, want nil", err)
	}

	if opts.Time != 1 {
		t.Fatalf("Time = %d, want 1", opts.Time)
	}

	if opts.Memory != DefaultMemory {
		t.Fatalf("Memory = %d, want %d", opts.Memory, DefaultMemory)
	}

	if opts.Threads != DefaultThreads {
		t.Fatalf("Threads = %d, want %d", opts.Threads, DefaultThreads)
	}

	if opts.SaltLen != DefaultSaltLen {
		t.Fatalf("SaltLen = %d, want %d", opts.SaltLen, DefaultSaltLen)
	}

	if opts.KeyLen != DefaultKeyLen {
		t.Fatalf("KeyLen = %d, want %d", opts.KeyLen, DefaultKeyLen)
	}
}

func TestOptionsValidate(t *testing.T) {
	t.Parallel()

	valid := Options{
		Time:    DefaultTime,
		Memory:  DefaultMemory,
		Threads: DefaultThreads,
		SaltLen: DefaultSaltLen,
		KeyLen:  DefaultKeyLen,
	}

	cases := []struct {
		name    string
		mutate  func(*Options)
		wantErr bool
	}{
		{name: "valid", mutate: func(*Options) {}, wantErr: false},
		{name: "time low", mutate: func(o *Options) { o.Time = 0 }, wantErr: true},
		{name: "time min", mutate: func(o *Options) { o.Time = 1 }, wantErr: false},
		{name: "time max", mutate: func(o *Options) { o.Time = 10 }, wantErr: false},
		{name: "time high", mutate: func(o *Options) { o.Time = 11 }, wantErr: true},
		{name: "memory low", mutate: func(o *Options) { o.Memory = 8*1024 - 1 }, wantErr: true},
		{name: "memory min", mutate: func(o *Options) { o.Memory = 8 * 1024 }, wantErr: false},
		{name: "memory max", mutate: func(o *Options) { o.Memory = 1024 * 1024 }, wantErr: false},
		{name: "memory high", mutate: func(o *Options) { o.Memory = 1024*1024 + 1 }, wantErr: true},
		{name: "threads low", mutate: func(o *Options) { o.Threads = 0 }, wantErr: true},
		{name: "threads min", mutate: func(o *Options) { o.Threads = 1 }, wantErr: false},
		{name: "threads max", mutate: func(o *Options) { o.Threads = 16 }, wantErr: false},
		{name: "threads high", mutate: func(o *Options) { o.Threads = 17 }, wantErr: true},
		{name: "salt low", mutate: func(o *Options) { o.SaltLen = 7 }, wantErr: true},
		{name: "salt min", mutate: func(o *Options) { o.SaltLen = 8 }, wantErr: false},
		{name: "salt max", mutate: func(o *Options) { o.SaltLen = 64 }, wantErr: false},
		{name: "salt high", mutate: func(o *Options) { o.SaltLen = 65 }, wantErr: true},
		{name: "key low", mutate: func(o *Options) { o.KeyLen = 15 }, wantErr: true},
		{name: "key min", mutate: func(o *Options) { o.KeyLen = 16 }, wantErr: false},
		{name: "key max", mutate: func(o *Options) { o.KeyLen = 128 }, wantErr: false},
		{name: "key high", mutate: func(o *Options) { o.KeyLen = 129 }, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := valid
			tc.mutate(&opts)

			err := opts.Validate()
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected non-nil error")
				}

				if !errors.Is(err, password.ErrInvalidHash) {
					t.Fatalf("errors.Is(%v, ErrInvalidHash) = false", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}
}

func TestNew(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		opts    password.Options
		wantErr bool
	}{
		{name: "defaults", opts: password.Options{}, wantErr: false},
		{
			name:    "custom valid",
			opts:    password.Options{Time: 1, Memory: 8 * 1024, Threads: 1, SaltLen: 8, KeyLen: 16},
			wantErr: false,
		},
		{name: "bad time", opts: password.Options{Time: 99}, wantErr: true},
		{name: "bad memory", opts: password.Options{Memory: 1}, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h, err := New(tc.opts)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected non-nil error")
				}

				if h != nil {
					t.Fatal("expected nil hasher")
				}

				if !errors.Is(err, password.ErrInvalidHash) {
					t.Fatalf("errors.Is(%v, ErrInvalidHash) = false", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("New() = %v, want nil", err)
			}

			if h == nil {
				t.Fatal("expected non-nil hasher")
			}
		})
	}
}
