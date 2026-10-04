package postgres

import (
	"errors"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/job"
	"github.com/zenta-dev/zever/core/scheduler"
)

func TestCoerceTime_table(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	nanos := now.Format(time.RFC3339Nano)

	cases := []struct {
		name    string
		in      any
		want    time.Time
		wantErr bool
	}{
		{"nil", nil, time.Time{}, false},
		{"time", now, now, false},
		{"string", nanos, now, false},
		{"bytes", []byte(nanos), now, false},
		{"int", 42, time.Time{}, true},
		{"bad string", "not-a-time", time.Time{}, true},
	}
	for _, tc := range cases {
		got, err := coerceTime(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("coerceTime(%v) err = nil, want error", tc.in)
			} else if !errors.Is(err, ErrUnsupportedType) && tc.name != "bad string" {
				t.Errorf("coerceTime(%v) err = %v, want ErrUnsupportedType", tc.in, err)
			}
			continue
		}
		if err != nil || !got.Equal(tc.want) {
			t.Errorf("coerceTime(%v) = (%v, %v), want %v", tc.in, got, err, tc.want)
		}
	}
}

func TestCoerceInt_table(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		in      any
		want    int64
		wantErr bool
	}{
		{"int64", int64(7), 7, false},
		{"int32", int32(7), 7, false},
		{"int", 7, 7, false},
		{"float64", float64(7), 7, false},
		{"string", "7", 0, true},
	}
	for _, tc := range cases {
		got, err := coerceInt(tc.in)
		if tc.wantErr {
			if err == nil || !errors.Is(err, ErrUnsupportedType) {
				t.Errorf("coerceInt(%v) = (%d, %v), want ErrUnsupportedType", tc.in, got, err)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("coerceInt(%v) = (%d, %v), want %d", tc.in, got, err, tc.want)
		}
	}
}

func TestOptions_Validate_table(t *testing.T) {
	t.Parallel()

	base := func() Options {
		o := Options{}
		o.Dispatcher = &job.Dispatcher{Q: newStubQueue()}
		return o
	}

	cases := []struct {
		name    string
		mutate  func(*Options)
		wantErr bool
	}{
		{"valid defaults", func(*Options) {}, false},
		{"negative lease", func(o *Options) { o.LeaseTTL = -time.Second }, true},
		{"negative fire timeout", func(o *Options) { o.FireTimeout = -time.Second }, true},
		{"bad table", func(o *Options) { o.Table = "bad-table" }, true},
		{"good table", func(o *Options) { o.Table = "sched_slots_2" }, false},
	}
	for _, tc := range cases {
		opts := base()
		tc.mutate(&opts)
		err := opts.Validate()
		if (err != nil) != tc.wantErr {
			t.Errorf("Validate(%s) = %v, wantErr=%v", tc.name, err, tc.wantErr)
		}
		if tc.wantErr && err != nil && tc.name == "negative fire timeout" && !errors.Is(err, scheduler.ErrInvalidOptions) {
			t.Errorf("Validate(%s) = %v, want scheduler.ErrInvalidOptions", tc.name, err)
		}
	}
}

func TestName(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{Owner: "owner-name"})
	t.Cleanup(func() { _ = d.Close() })
	if got := d.Name(); got != "postgres" {
		t.Errorf("Name() = %q, want postgres", got)
	}
}
