package geo

import (
	"sync"
	"testing"
	"time"
)

func TestGeoEdge_OpenConcurrent(t *testing.T) {
	adapter := geoFreshAdapter()
	if err := Register(adapter, func(Options) (Geo, error) { return &stubGeo{}, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	var wg sync.WaitGroup

	for range 50 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			if _, err := Open(adapter, Options{}); err != nil {
				t.Errorf("Open err = %v", err)
			}
		}()
	}

	wg.Wait()
}

func TestGeoEdge_RegisterConcurrentUnique(t *testing.T) {
	var wg sync.WaitGroup

	errs := make([]error, 20)

	for i := range errs {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			errs[i] = Register(geoFreshAdapter(), func(Options) (Geo, error) { return &stubGeo{}, nil })
		}(i)
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("Register worker %d err = %v", i, err)
		}
	}
}

func TestGeoEdge_OptionsValidateBoundary(t *testing.T) {
	tests := []struct {
		name    string
		opts    Options
		wantErr bool
	}{
		{name: "zero valid", opts: Options{}},
		{name: "timeout zero valid", opts: Options{Timeout: 0}},
		{name: "max body zero valid", opts: Options{MaxResponseBody: 0}},
		{name: "timeout negative invalid", opts: Options{Timeout: -time.Second}, wantErr: true},
		{name: "max body negative invalid", opts: Options{MaxResponseBody: -1}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.opts.Validate() != nil; got != tt.wantErr {
				t.Fatalf("Validate() error present = %v, want %v", got, tt.wantErr)
			}
		})
	}
}
