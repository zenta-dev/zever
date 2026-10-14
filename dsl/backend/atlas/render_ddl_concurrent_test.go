package atlas

import (
	"strings"
	"testing"
)

func TestRenderSchemaDDLConcurrentIndexes(t *testing.T) {
	t.Parallel()

	src := `entity User {
		id: uuid @primary
		email: string @unique
		nickname: string

		index(nickname)
	}`

	plain, err := RenderSchemaDDL(DialectPostgres, compileSchema(t, src))
	if err != nil {
		t.Fatalf("RenderSchemaDDL: %v", err)
	}

	for _, s := range plain {
		if strings.Contains(s, "CONCURRENTLY") {
			t.Fatalf("default render must not emit CONCURRENTLY: %q", s)
		}
	}

	concurrent, err := RenderSchemaDDLWithOptions(DialectPostgres, compileSchema(t, src), DDLOptions{ConcurrentIndexes: true})
	if err != nil {
		t.Fatalf("RenderSchemaDDLWithOptions: %v", err)
	}

	found := false

	for _, s := range concurrent {
		if strings.Contains(s, "CREATE") && strings.Contains(s, "INDEX") {
			if !strings.Contains(s, "CONCURRENTLY") {
				t.Fatalf("concurrent render missing CONCURRENTLY: %q", s)
			}

			found = true
		}
	}

	if !found {
		t.Fatalf("expected at least one index statement, got %v", concurrent)
	}
}

func TestRenderSchemaDDLConcurrentIgnoredOffPostgres(t *testing.T) {
	t.Parallel()

	src := `entity User {
		id: uuid @primary
		nickname: string

		index(nickname)
	}`

	stmts, err := RenderSchemaDDLWithOptions(DialectSQLite, compileSchema(t, src), DDLOptions{ConcurrentIndexes: true})
	if err != nil {
		t.Fatalf("RenderSchemaDDLWithOptions: %v", err)
	}

	for _, s := range stmts {
		if strings.Contains(s, "CONCURRENTLY") {
			t.Fatalf("non-postgres render must not emit CONCURRENTLY: %q", s)
		}
	}
}
