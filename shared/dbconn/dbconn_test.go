package dbconn

import (
	"errors"
	"strings"
	"testing"
)

func TestIsPostgresDSN(t *testing.T) {
	t.Parallel()

	tests := []struct {
		dsn  string
		want bool
	}{
		{"", false},
		{":memory:", false},
		{"file.db", false},
		{"/tmp/x.db", false},
		{"queue.db", false},
		{"postgres://localhost:5432/zever", true},
		{"postgresql://localhost:5432/zever", true},
		{"postgres://user:pass@localhost:5432/q?sslmode=disable", true},
		{"  POSTGRES://h/db  ", true},
		{"mysql://h/db", false},
		{"://bad", false},
	}

	for _, tt := range tests {
		t.Run(tt.dsn, func(t *testing.T) {
			t.Parallel()

			if got := IsPostgresDSN(tt.dsn); got != tt.want {
				t.Errorf("IsPostgresDSN(%q) = %v, want %v", tt.dsn, got, tt.want)
			}
		})
	}
}

func TestSplitDSN(t *testing.T) {
	t.Parallel()

	if got := SplitDSN(""); got.Path != ":memory:" || got.DSN != "" {
		t.Errorf("SplitDSN(\"\") = %+v, want sqlite :memory:", got)
	}

	if got := SplitDSN("file.db"); got.Path != "file.db" || got.DSN != "" {
		t.Errorf("SplitDSN(file.db) = %+v, want Path", got)
	}

	if got := SplitDSN("postgres://h/db"); got.DSN != "postgres://h/db" || got.Path != "" {
		t.Errorf("SplitDSN(postgres) = %+v, want DSN", got)
	}
}

func TestValidateTableName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		wantErr bool
	}{
		{"queue_messages", false},
		{"_leading", false},
		{"with_digit_1", false},
		{"", true},
		{"queue-messages", true},
		{"no-dashes!", true},
		{"1leading_digit", true},
		{"with space", true},
	}

	for _, tt := range tests {
		t.Run("name="+tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateTableName(tt.name)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateTableName(%q) err = %v, wantErr %v", tt.name, err, tt.wantErr)
			}

			if err != nil && !errors.Is(err, ErrInvalidTableName) {
				t.Errorf("ValidateTableName(%q) err = %v, want ErrInvalidTableName", tt.name, err)
			}
		})
	}
}

func TestRandomOwner(t *testing.T) {
	a := RandomOwner("queue-owner")
	b := RandomOwner("queue-owner")

	if a == "" || b == "" {
		t.Fatal("RandomOwner returned empty owner")
	}

	if !strings.HasPrefix(a, "queue-owner-") || !strings.HasPrefix(b, "queue-owner-") {
		t.Fatalf("RandomOwner owners = %q, %q, want queue-owner- prefix", a, b)
	}

	if a == b {
		t.Fatal("RandomOwner minted identical owners for two drivers")
	}
}
