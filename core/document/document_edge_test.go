package document

import (
	"testing"
	"time"
)

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
