package media

import (
	"errors"
	"testing"
	"time"
)

func TestOptionsValidate_presignTTLBoundaries(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		ttl     time.Duration
		wantErr bool
	}{
		{name: "zero valid", ttl: 0, wantErr: false},
		{name: "max valid", ttl: MaxPresignTTL, wantErr: false},
		{name: "max plus one invalid", ttl: MaxPresignTTL + time.Nanosecond, wantErr: true},
		{name: "negative invalid", ttl: -time.Second, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := Options{PresignTTL: tc.ttl}.Validate()
			if tc.wantErr && !errors.Is(err, ErrInvalidOptions) {
				t.Fatalf("PresignTTL=%v err = %v, want ErrInvalidOptions", tc.ttl, err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("PresignTTL=%v err = %v, want nil", tc.ttl, err)
			}
		})
	}
}
