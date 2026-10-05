package db

import (
	"os"
	"testing"
)

// postgresDSN returns POSTGRES_DSN or skips the test.
func postgresDSN(t *testing.T) string {
	t.Helper()

	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set POSTGRES_DSN to run postgres live tests")
	}

	return dsn
}
