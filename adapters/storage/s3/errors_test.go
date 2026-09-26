package s3

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/storage"
)

func TestSentinelMessages(t *testing.T) {
	t.Parallel()
	tests := []struct {
		err  error
		want string
	}{
		{err: ErrInvalidOption, want: "s3: invalid option"},
	}
	for _, tt := range tests {
		if got := tt.err.Error(); got != tt.want {
			t.Errorf("Error() = %q, want %q", got, tt.want)
		}
	}
}

func TestNewInvalidOptions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		opts storage.Options
	}{
		{name: "endpoint without url_base", opts: storage.Options{Endpoint: "http://localhost:9000"}},
		{name: "policy_sync bogus", opts: storage.Options{PolicySync: "bogus"}},
		{name: "sync_fail bogus", opts: storage.Options{SyncFail: "bogus"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := New(tt.opts)
			if err == nil {
				t.Fatal("New() = nil, want ErrInvalidOption")
			}
			if !errors.Is(err, ErrInvalidOption) {
				t.Fatalf("errors.Is(err, ErrInvalidOption) = false (err = %v)", err)
			}
		})
	}
}
