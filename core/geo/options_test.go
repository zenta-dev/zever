package geo

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestOptionsValidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		opts      Options
		wantErr   bool
		wantIs    error
		contains  []string
		wantCount int // number of joined errors, 0 = don't check
	}{
		{name: "happy empty", opts: Options{}},
		{name: "happy full https", opts: Options{
			Timeout:         time.Second,
			MaxResponseBody: 1024,
			BaseURL:         "https://maps.example.com",
			Endpoint:        "https://osm.example.com",
		}},
		{name: "https passes", opts: Options{BaseURL: "https://example.com/api", Endpoint: "https://example.com/nominatim"}},
		{name: "http insecure passes", opts: Options{
			BaseURL:       "http://localhost:8080",
			Endpoint:      "http://localhost:8081",
			AllowInsecure: true,
		}},
		{name: "negative timeout", opts: Options{Timeout: -1}, wantErr: true, wantIs: ErrInvalidOptions, contains: []string{"timeout"}},
		{name: "negative max body", opts: Options{MaxResponseBody: -1}, wantErr: true, wantIs: ErrInvalidOptions, contains: []string{"max_response_body"}},
		{name: "baseurl no scheme", opts: Options{BaseURL: "example.com/api"}, wantErr: true, wantIs: ErrInvalidOptions, contains: []string{"base_url"}},
		{name: "baseurl no host", opts: Options{BaseURL: "https://"}, wantErr: true, wantIs: ErrInvalidOptions, contains: []string{"base_url"}},
		{name: "baseurl http locked", opts: Options{BaseURL: "http://example.com"}, wantErr: true, wantIs: ErrInvalidOptions, contains: []string{"base_url must use https"}},
		{name: "baseurl bad scheme", opts: Options{BaseURL: "ftp://example.com"}, wantErr: true, wantIs: ErrInvalidOptions, contains: []string{"base_url must use https"}},
		{name: "endpoint no scheme", opts: Options{Endpoint: "example.com/api"}, wantErr: true, wantIs: ErrInvalidOptions, contains: []string{"endpoint"}},
		{name: "endpoint no host", opts: Options{Endpoint: "https://"}, wantErr: true, wantIs: ErrInvalidOptions, contains: []string{"endpoint"}},
		{name: "endpoint http locked", opts: Options{Endpoint: "http://example.com"}, wantErr: true, wantIs: ErrInvalidOptions, contains: []string{"endpoint must use https"}},
		{name: "endpoint bad scheme", opts: Options{Endpoint: "ftp://example.com"}, wantErr: true, wantIs: ErrInvalidOptions, contains: []string{"endpoint must use https"}},
		{
			name:      "multiple joined",
			opts:      Options{Timeout: -1, MaxResponseBody: -1, BaseURL: "http://example.com", Endpoint: "http://example.com"},
			wantErr:   true,
			wantIs:    ErrInvalidOptions,
			wantCount: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.opts.Validate()
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("expected nil, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Fatalf("expected errors.Is(%v), got %v", tt.wantIs, err)
			}
			for _, sub := range tt.contains {
				if !strings.Contains(err.Error(), sub) {
					t.Fatalf("expected error to contain %q, got %v", sub, err)
				}
			}
			if tt.wantCount > 0 {
				var invErr *InvalidOptionsError
				var joinErr interface{ Unwrap() []error }
				if !errors.As(err, &joinErr) {
					t.Fatalf("expected join error, got %T", err)
				}
				n := 0
				for _, e := range joinErr.Unwrap() {
					if errors.As(e, &invErr) {
						n++
					}
				}
				if n != tt.wantCount {
					t.Fatalf("expected %d joined InvalidOptionsError, got %d: %v", tt.wantCount, n, err)
				}
			}
		})
	}
}
