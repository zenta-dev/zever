package idempotency

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestIdempotencyEdge_KeyBoundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{name: "one valid", key: "a"},
		{name: "max valid", key: strings.Repeat("a", MaxKeyLen)},
		{name: "empty invalid", key: "", wantErr: true},
		{name: "too long invalid", key: strings.Repeat("a", MaxKeyLen+1), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateKey(tt.key)
			if tt.wantErr != (err != nil) {
				t.Fatalf("ValidateKey() err = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr {
				if !errors.Is(err, ErrInvalidKey) {
					t.Fatalf("errors.Is(err, ErrInvalidKey) = false (err = %v)", err)
				}

				got, ok := errors.AsType[InvalidKeyError](err)
				if !ok || got.KeyLen != len(tt.key) {
					t.Fatalf("errors.AsType[InvalidKeyError] = %+v, %v; want KeyLen %d", got, ok, len(tt.key))
				}
			}
		})
	}
}

func TestIdempotencyEdge_TTLDefault(t *testing.T) {
	t.Parallel()

	if got := (Options{}).ttl(); got != DefaultTTL {
		t.Fatalf("zero Options ttl() = %v, want %v", got, DefaultTTL)
	}
}

func TestIdempotencyEdge_OptionsValidateBoundary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		opts    Options
		wantErr bool
	}{
		{name: "zero valid", opts: Options{}},
		{name: "ttl zero valid", opts: Options{TTL: 0}},
		{name: "negative max entries valid", opts: Options{MaxEntries: -1}},
		{name: "ttl negative invalid", opts: Options{TTL: -time.Second}, wantErr: true},
		{name: "bad redis addr invalid", opts: Options{Redis: RedisOptions{Addr: "not-an-addr"}}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.opts.Validate() != nil; got != tt.wantErr {
				t.Fatalf("Validate() error present = %v, want %v", got, tt.wantErr)
			}
		})
	}
}
