package geo

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// stubGeo is a minimal Geo implementation for facade tests.
type stubGeo struct{}

func (s *stubGeo) Geocode(_ context.Context, _ string) ([]Location, error) {
	return []Location{{Lat: 1, Lng: 2, Formatted: "stub"}}, nil
}

func (s *stubGeo) ReverseGeocode(_ context.Context, _, _ float64) ([]Address, error) {
	return []Address{{Formatted: "stub"}}, nil
}

func (s *stubGeo) Distance(_ context.Context, _, _ Point) (float64, error) {
	return 42, nil
}

func (s *stubGeo) Close() error {
	return nil
}

// NOTE: no t.Parallel anywhere in this file: registry is a global map.

func TestRegister(t *testing.T) {
	tests := []struct {
		name    string
		adapter Adapter
		factory Factory
		wantIs  error
		wantAs  any
		wantErr bool
	}{
		{
			name:    "nil factory",
			adapter: Adapter(90),
			factory: nil,
			wantIs:  ErrNilFactory,
			wantErr: true,
		},
		{
			name:    "success",
			adapter: Adapter(91),
			factory: func(Options) (Geo, error) { return &stubGeo{}, nil },
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Register(tt.adapter, tt.factory)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
					t.Fatalf("expected errors.Is(%v), got %v", tt.wantIs, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected nil, got %v", err)
			}
		})
	}
}

func TestRegisterDuplicate(t *testing.T) {
	const dup = Adapter(99)

	if err := Register(dup, func(Options) (Geo, error) { return &stubGeo{}, nil }); err != nil {
		t.Fatalf("setup Register failed: %v", err)
	}

	err := Register(dup, func(Options) (Geo, error) { return &stubGeo{}, nil })
	if err == nil {
		t.Fatal("expected duplicate error, got nil")
	}
	if !errors.Is(err, ErrDuplicateAdapter) {
		t.Fatalf("expected errors.Is ErrDuplicateAdapter, got %v", err)
	}
	var dupErr *DuplicateAdapterError
	if !errors.As(err, &dupErr) {
		t.Fatalf("expected errors.As DuplicateAdapterError, got %T", err)
	}
	if dupErr.Adapter != dup {
		t.Fatalf("expected adapter %v, got %v", dup, dupErr.Adapter)
	}
}

func TestOpen(t *testing.T) {
	t.Run("validate failure", func(t *testing.T) {
		_, err := Open(Adapter(92), Options{Timeout: -1})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrInvalidOptions) {
			t.Fatalf("expected errors.Is ErrInvalidOptions, got %v", err)
		}
	})

	t.Run("unknown adapter", func(t *testing.T) {
		const unknown = Adapter(93)
		_, err := Open(unknown, Options{})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrUnknownAdapter) {
			t.Fatalf("expected errors.Is ErrUnknownAdapter, got %v", err)
		}
		var ue *UnknownAdapterError
		if !errors.As(err, &ue) {
			t.Fatalf("expected errors.As UnknownAdapterError, got %T", err)
		}
		if !strings.Contains(err.Error(), "forgotten import?") {
			t.Fatalf("expected 'forgotten import?' hint, got %v", err)
		}
	})

	t.Run("factory error wrapped", func(t *testing.T) {
		const bad = Adapter(94)
		sentinel := errors.New("boom")
		if err := Register(bad, func(Options) (Geo, error) { return nil, sentinel }); err != nil {
			t.Fatalf("setup Register failed: %v", err)
		}
		_, err := Open(bad, Options{})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, sentinel) {
			t.Fatalf("expected errors.Is sentinel, got %v", err)
		}
		if !strings.Contains(err.Error(), "geo: open") {
			t.Fatalf("expected 'geo: open' prefix, got %v", err)
		}
	})

	t.Run("success", func(t *testing.T) {
		const ok = Adapter(95)
		if err := Register(ok, func(Options) (Geo, error) { return &stubGeo{}, nil }); err != nil {
			t.Fatalf("setup Register failed: %v", err)
		}
		g, err := Open(ok, Options{})
		if err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
		if g == nil {
			t.Fatal("expected non-nil Geo")
		}
		ctx := t.Context()
		locs, err := g.Geocode(ctx, "x")
		if err != nil || len(locs) != 1 {
			t.Fatalf("Geocode = %v, %v; want 1 location", locs, err)
		}
		addrs, err := g.ReverseGeocode(ctx, 0, 0)
		if err != nil || len(addrs) != 1 {
			t.Fatalf("ReverseGeocode = %v, %v; want 1 address", addrs, err)
		}
		d, err := g.Distance(ctx, Point{}, Point{})
		if err != nil || d != 42 {
			t.Fatalf("Distance = %v, %v; want 42", d, err)
		}
		if err := g.Close(); err != nil {
			t.Fatalf("Close = %v; want nil", err)
		}
	})
}
