package document

import (
	"sync"
	"testing"
	"time"
)

func TestDocumentEdge_OpenConcurrent(t *testing.T) {
	t.Parallel()

	adapter := freshAdapter()
	if err := Register(adapter, func(Options) (Document, error) { return &stubDocument{}, nil }); err != nil {
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

func TestDocumentEdge_RegisterConcurrentUnique(t *testing.T) {
	t.Parallel()

	var wg sync.WaitGroup

	errs := make([]error, 20)

	for i := range errs {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			errs[i] = Register(freshAdapter(), func(Options) (Document, error) { return &stubDocument{}, nil })
		}(i)
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("Register worker %d err = %v", i, err)
		}
	}
}

func TestDocumentEdge_OptionsValidateBoundary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		opts    Options
		wantErr bool
	}{
		{name: "zero valid", opts: Options{}},
		{name: "quality zero valid", opts: Options{Quality: 0}},
		{name: "quality hundred valid", opts: Options{Quality: 100}},
		{name: "timeout zero valid", opts: Options{Timeout: 0}},
		{name: "quality negative invalid", opts: Options{Quality: -1}, wantErr: true},
		{name: "quality high invalid", opts: Options{Quality: 101}, wantErr: true},
		{name: "timeout negative invalid", opts: Options{Timeout: -time.Second}, wantErr: true},
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
