package password_test

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/password"
)

func validOptions() password.Options {
	return password.Options{
		Time:    password.DefaultTime,
		Memory:  password.DefaultMemory,
		Threads: password.DefaultThreads,
		SaltLen: password.DefaultSaltLen,
		KeyLen:  password.DefaultKeyLen,
	}
}

func TestOptionsValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		mutate  func(*password.Options)
		wantErr bool
	}{
		{name: "valid defaults", mutate: func(*password.Options) {}, wantErr: false},
		{name: "zero fails", mutate: func(o *password.Options) { *o = password.Options{} }, wantErr: true},
		{name: "time low", mutate: func(o *password.Options) { o.Time = 0 }, wantErr: true},
		{name: "time min", mutate: func(o *password.Options) { o.Time = 1 }, wantErr: false},
		{name: "time max", mutate: func(o *password.Options) { o.Time = 10 }, wantErr: false},
		{name: "time high", mutate: func(o *password.Options) { o.Time = 11 }, wantErr: true},
		{name: "memory low", mutate: func(o *password.Options) { o.Memory = 8*1024 - 1 }, wantErr: true},
		{name: "memory min", mutate: func(o *password.Options) { o.Memory = 8 * 1024 }, wantErr: false},
		{name: "memory max", mutate: func(o *password.Options) { o.Memory = 1024 * 1024 }, wantErr: false},
		{name: "memory high", mutate: func(o *password.Options) { o.Memory = 1024*1024 + 1 }, wantErr: true},
		{name: "threads low", mutate: func(o *password.Options) { o.Threads = 0 }, wantErr: true},
		{name: "threads min", mutate: func(o *password.Options) { o.Threads = 1 }, wantErr: false},
		{name: "threads max", mutate: func(o *password.Options) { o.Threads = 16 }, wantErr: false},
		{name: "threads high", mutate: func(o *password.Options) { o.Threads = 17 }, wantErr: true},
		{name: "salt low", mutate: func(o *password.Options) { o.SaltLen = 7 }, wantErr: true},
		{name: "salt min", mutate: func(o *password.Options) { o.SaltLen = 8 }, wantErr: false},
		{name: "salt max", mutate: func(o *password.Options) { o.SaltLen = 64 }, wantErr: false},
		{name: "salt high", mutate: func(o *password.Options) { o.SaltLen = 65 }, wantErr: true},
		{name: "key low", mutate: func(o *password.Options) { o.KeyLen = 15 }, wantErr: true},
		{name: "key min", mutate: func(o *password.Options) { o.KeyLen = 16 }, wantErr: false},
		{name: "key max", mutate: func(o *password.Options) { o.KeyLen = 128 }, wantErr: false},
		{name: "key high", mutate: func(o *password.Options) { o.KeyLen = 129 }, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := validOptions()
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

func TestOptionDefaults(t *testing.T) {
	t.Parallel()

	if password.DefaultTime != 3 {
		t.Fatalf("DefaultTime = %d, want 3", password.DefaultTime)
	}

	if password.DefaultMemory != 64*1024 {
		t.Fatalf("DefaultMemory = %d, want %d", password.DefaultMemory, 64*1024)
	}

	if password.DefaultThreads != 4 {
		t.Fatalf("DefaultThreads = %d, want 4", password.DefaultThreads)
	}

	if password.DefaultSaltLen != 16 {
		t.Fatalf("DefaultSaltLen = %d, want 16", password.DefaultSaltLen)
	}

	if password.DefaultKeyLen != 32 {
		t.Fatalf("DefaultKeyLen = %d, want 32", password.DefaultKeyLen)
	}
}
