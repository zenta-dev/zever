package retry

import (
	"testing"
	"time"
)

func TestParseRetryAfterDelaySeconds(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name   string
		header string
		want   time.Duration
	}{
		{"integer seconds", "120", 120 * time.Second},
		{"zero seconds", "0", 0},
		{"float seconds", "1.5", 1500 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseRetryAfter(tt.header, now)
			if !ok {
				t.Fatalf("ParseRetryAfter(%q) ok = false, want true", tt.header)
			}

			if got != tt.want {
				t.Errorf("ParseRetryAfter(%q) = %v, want %v", tt.header, got, tt.want)
			}
		})
	}
}

func TestParseRetryAfterHTTPDate(t *testing.T) {
	now := time.Date(2026, time.October, 21, 7, 26, 0, 0, time.UTC)
	header := "Wed, 21 Oct 2026 07:28:00 GMT"

	got, ok := ParseRetryAfter(header, now)
	if !ok {
		t.Fatalf("ParseRetryAfter(%q) ok = false, want true", header)
	}

	want := 2 * time.Minute
	if got != want {
		t.Errorf("ParseRetryAfter(%q) = %v, want %v", header, got, want)
	}
}

func TestParseRetryAfterHTTPDateInPast(t *testing.T) {
	now := time.Date(2026, time.October, 21, 7, 30, 0, 0, time.UTC)
	header := "Wed, 21 Oct 2026 07:28:00 GMT"

	got, ok := ParseRetryAfter(header, now)
	if !ok {
		t.Fatalf("ParseRetryAfter(%q) ok = false, want true", header)
	}

	if got != 0 {
		t.Errorf("ParseRetryAfter(%q) = %v, want 0 (already elapsed)", header, got)
	}
}

func TestParseRetryAfterInvalid(t *testing.T) {
	now := time.Now()

	tests := []string{"", "not-a-date", "abc", "Retry-After: banana"}

	for _, header := range tests {
		if _, ok := ParseRetryAfter(header, now); ok {
			t.Errorf("ParseRetryAfter(%q) ok = true, want false", header)
		}
	}
}

func TestParseRetryAfterMs(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   time.Duration
		wantOK bool
	}{
		{"integer ms", "1500", 1500 * time.Millisecond, true},
		{"float ms", "250.5", time.Duration(250.5 * float64(time.Millisecond)), true},
		{"zero", "0", 0, true},
		{"empty", "", 0, false},
		{"invalid", "not-a-number", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseRetryAfterMs(tt.header)
			if ok != tt.wantOK {
				t.Fatalf("ParseRetryAfterMs(%q) ok = %v, want %v", tt.header, ok, tt.wantOK)
			}

			if ok && got != tt.want {
				t.Errorf("ParseRetryAfterMs(%q) = %v, want %v", tt.header, got, tt.want)
			}
		})
	}
}

func TestParseRetryAfterMsNegativeClampedToZero(t *testing.T) {
	got, ok := ParseRetryAfterMs("-50")
	if !ok {
		t.Fatal("ParseRetryAfterMs(-50) ok = false, want true")
	}

	if got != 0 {
		t.Errorf("ParseRetryAfterMs(-50) = %v, want 0", got)
	}
}
