package geo

import (
	"testing"
	"time"
)

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
