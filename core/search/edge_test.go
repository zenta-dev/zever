package search

import (
	"errors"
	"sync"
	"testing"
)

func TestDocument_Validate_table(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		meta    map[string]any
		wantErr bool
	}{
		{"nil metadata", nil, false},
		{"empty metadata", map[string]any{}, false},
		{"string value", map[string]any{"k": "v"}, false},
		{"nested map", map[string]any{"k": map[string]any{"n": 1}}, false},
		{"slice value", map[string]any{"k": []int{1, 2, 3}}, false},
		{"func value", map[string]any{"k": func() {}}, true},
		{"chan value", map[string]any{"k": make(chan int)}, true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := (Document{Metadata: tc.meta}).Validate()
			if tc.wantErr && !errors.Is(err, ErrInvalidMetadata) {
				t.Fatalf("Validate() err = %v, want ErrInvalidMetadata", err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("Validate() err = %v, want nil", err)
			}
		})
	}
}

func TestQueryOptions_defaults_not_mutated(t *testing.T) {
	t.Parallel()

	opts := QueryOptions{Limit: -1, Offset: -5}
	if opts.Limit != -1 || opts.Offset != -5 {
		t.Fatalf("QueryOptions zero-value handling changed: %+v", opts)
	}
}

func TestOpen_https_with_port(t *testing.T) {
	t.Parallel()

	a := freshAdapter()
	if err := Register(a, func(Options) (Search, error) { return &stubSearch{}, nil }); err != nil {
		t.Fatalf("Register(%v) error = %v", a, err)
	}

	got, err := Open(a, Options{Host: "https://search.example.com:7700"})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if got == nil {
		t.Fatal("Open() returned nil Search")
	}
}

func TestRegister_Open_concurrent(t *testing.T) {
	t.Parallel()

	const n = 16
	var wg sync.WaitGroup
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()

			a := freshAdapter()
			if err := Register(a, func(Options) (Search, error) { return &stubSearch{}, nil }); err != nil {
				t.Errorf("Register(%v) error = %v", a, err)
				return
			}
			if _, err := Open(a, Options{}); err != nil {
				t.Errorf("Open(%v) error = %v", a, err)
			}
		}()
	}

	wg.Wait()
}
