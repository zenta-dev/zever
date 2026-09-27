package observability

import "testing"

func TestSentinelMessages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		err  error
		want string
	}{
		{ErrNilFactory, "observability: nil factory"},
		{ErrDuplicate, "observability: duplicate registration"},
		{ErrUnknownAdapter, "observability: unknown adapter"},
		{ErrInvalidAdapter, "observability: invalid adapter"},
		{ErrInvalidOptions, "observability: invalid options"},
		{ErrTooManyInstruments, "observability: too many instruments"},
		{ErrInstrumentConflict, "observability: instrument conflict"},
	}

	for _, tt := range tests {
		if got := tt.err.Error(); got != tt.want {
			t.Errorf("sentinel message = %q, want %q", got, tt.want)
		}
	}
}
