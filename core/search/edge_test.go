package search

import (
	"errors"
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

