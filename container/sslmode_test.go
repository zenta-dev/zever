package container

import (
	"strings"
	"testing"
)

func TestPostgresSSLModeWarning(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		dsn  string
		env  map[string]string
		warn bool
	}{
		{"prefer remote url", "postgres://u:p@db.example.com:5432/app?sslmode=prefer", nil, true},
		{"unset remote url defaults to prefer", "postgres://u:p@db.example.com/app", nil, true},
		{"allow remote", "postgres://u@db.example.com/app?sslmode=allow", nil, true},
		{"keyword form prefer", "host=db.example.com user=u dbname=app sslmode=prefer", nil, true},
		{"env prefer", "postgres://u@db.example.com/app", map[string]string{"PGSSLMODE": "prefer"}, true},
		{"require ok", "postgres://u@db.example.com/app?sslmode=require", nil, false},
		{"verify-full ok", "postgres://u@db.example.com/app?sslmode=verify-full", nil, false},
		{"disable is explicit opt-out", "postgres://u@db.example.com/app?sslmode=disable", nil, false},
		{"env disable wins when dsn silent", "postgres://u@db.example.com/app", map[string]string{"PGSSLMODE": "disable"}, false},
		{"localhost ok", "postgres://u@localhost/app", nil, false},
		{"loopback ip ok", "postgres://u@127.0.0.1:5432/app?sslmode=prefer", nil, false},
		{"ipv6 loopback ok", "postgres://u@[::1]:5432/app", nil, false},
		{"unix socket ok", "host=/var/run/postgresql dbname=app", nil, false},
		{"no host ok", "dbname=app", nil, false},
		{"mixed hosts warn", "postgres://u@localhost,db.example.com/app", nil, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := postgresSSLModeWarning(tc.dsn, func(k string) string { return tc.env[k] })
			if (got != "") != tc.warn {
				t.Fatalf("warning = %q, want warn=%v", got, tc.warn)
			}

			if strings.Contains(got, "u:p") || strings.Contains(got, "example.com") {
				t.Errorf("warning leaks DSN content: %q", got)
			}
		})
	}
}
