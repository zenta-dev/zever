package db

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/idempotency"
)

// mustDriver opens a fresh file-backed store and returns it as *driver so
// edge tests can seed raw kv records.
func mustDriver(t *testing.T) *driver {
	t.Helper()

	s := mustNew(t)

	d, ok := s.(*driver)
	if !ok {
		t.Fatalf("New type = %T, want *driver", s)
	}

	return d
}

func TestEdge_DecodeWire(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		raw      []byte
		wantTag  byte
		wantFP   string
		wantRes  string
		wantCorr bool
	}{
		"pending":    {raw: encodePending([]byte("fp")), wantTag: tagPending, wantFP: "fp"},
		"done":       {raw: encodeDone([]byte("fp"), []byte("res")), wantTag: tagDone, wantFP: "fp", wantRes: "res"},
		"done empty": {raw: encodeDone(nil, nil), wantTag: tagDone},
		"empty":      {raw: []byte{}, wantCorr: true},
		"short":      {raw: []byte("x"), wantCorr: true},
		"bad tag":    {raw: []byte{'X', 0, 0}, wantCorr: true},
		"truncated":  {raw: []byte{tagPending, 0, 10, 'a'}, wantCorr: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			tag, fp, result, err := decode(tc.raw)
			if tc.wantCorr {
				if !errors.Is(err, idempotency.ErrCorruptRecord) {
					t.Fatalf("decode(%s) err = %v, want ErrCorruptRecord", name, err)
				}

				return
			}

			if err != nil {
				t.Fatalf("decode(%s) err = %v, want nil", name, err)
			}

			if tag != tc.wantTag || string(fp) != tc.wantFP || string(result) != tc.wantRes {
				t.Fatalf("decode(%s) = (%q, %q, %q), want (%q, %q, %q)", name, tag, fp, result, tc.wantTag, tc.wantFP, tc.wantRes)
			}
		})
	}
}

func TestEdge_BeginCorruptRecordFailsClosed(t *testing.T) {
	t.Parallel()

	d := mustDriver(t)
	ctx := t.Context()

	if err := d.kv.Set(ctx, d.kvKey("k"), []byte("not-a-record"), time.Minute); err != nil {
		t.Fatalf("seed failed: %v", err)
	}

	if _, err := d.Begin(ctx, "k", idempotency.BeginOptions{}); !errors.Is(err, idempotency.ErrCorruptRecord) {
		t.Fatalf("Begin corrupt = %v, want ErrCorruptRecord", err)
	}
}

func TestEdge_CompleteCorruptRecordFailsClosed(t *testing.T) {
	t.Parallel()

	d := mustDriver(t)
	ctx := t.Context()

	if err := d.kv.Set(ctx, d.kvKey("k"), []byte("not-a-record"), time.Minute); err != nil {
		t.Fatalf("seed failed: %v", err)
	}

	if err := d.Complete(ctx, "k", nil, []byte("res")); !errors.Is(err, idempotency.ErrCorruptRecord) {
		t.Fatalf("Complete corrupt = %v, want ErrCorruptRecord", err)
	}
}

func TestEdge_ContextCancelled(t *testing.T) {
	t.Parallel()

	s := mustNew(t)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := s.Begin(ctx, "k", idempotency.BeginOptions{}); !errors.Is(err, context.Canceled) {
		t.Errorf("Begin cancelled = %v, want context.Canceled", err)
	}

	if err := s.Complete(ctx, "k", nil, nil); !errors.Is(err, context.Canceled) {
		t.Errorf("Complete cancelled = %v, want context.Canceled", err)
	}

	if err := s.Forget(ctx, "k"); !errors.Is(err, context.Canceled) {
		t.Errorf("Forget cancelled = %v, want context.Canceled", err)
	}
}

func TestEdge_CloseOwnedClosesConnection(t *testing.T) {
	t.Parallel()

	d := mustDriver(t)

	if err := d.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if err := d.db.Ping(context.Background()); err == nil {
		t.Fatal("Ping after Close = nil, want closed-connection error")
	}
}

func TestEdge_NewInvalidOptions(t *testing.T) {
	t.Parallel()

	_, err := New(Options{TTL: -time.Second})
	if err == nil {
		t.Fatal("New(negative TTL) = nil, want error")
	}

	if !strings.HasPrefix(err.Error(), "db: ") {
		t.Errorf("New(negative TTL) = %q, want db: prefix", err)
	}
}
