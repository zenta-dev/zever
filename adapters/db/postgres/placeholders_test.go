package postgres

import (
	"fmt"
	"sync"
	"testing"
)

func TestReplacePlaceholders(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		query string
		want  string
	}{
		{
			name:  "single placeholder",
			query: "SELECT id FROM sessions WHERE id = ?",
			want:  "SELECT id FROM sessions WHERE id = $1",
		},
		{
			name:  "multiple placeholders",
			query: "INSERT INTO t (a, b) VALUES (?, ?)",
			want:  "INSERT INTO t (a, b) VALUES ($1, $2)",
		},
		{
			name:  "placeholders with positional ordering",
			query: "WHERE x = ? AND y IN (?, ?)",
			want:  "WHERE x = $1 AND y IN ($2, $3)",
		},
		{
			name:  "question mark in string literal ignored",
			query: "UPDATE t SET s = 'why?' WHERE id = ?",
			want:  "UPDATE t SET s = 'why?' WHERE id = $1",
		},
		{
			name:  "doubled quote escape in string literal",
			query: "SELECT 'it''s ?' , ?",
			want:  "SELECT 'it''s ?' , $1",
		},
		{
			name:  "question mark in double-quoted identifier ignored",
			query: `SELECT "col?" FROM t WHERE id = ?`,
			want:  `SELECT "col?" FROM t WHERE id = $1`,
		},
		{
			name:  "question mark in dollar-quoted string ignored",
			query: "SELECT $$why?$$ AS s, ?",
			want:  "SELECT $$why?$$ AS s, $1",
		},
		{
			name:  "question mark in tagged dollar string ignored",
			query: `INSERT INTO t (js) VALUES ($json${"a": "?"}$json$), (?)`,
			want:  `INSERT INTO t (js) VALUES ($json${"a": "?"}$json$), ($1)`,
		},
		{
			name:  "dollar quote spans to end of query",
			query: "SELECT $$a?b$$",
			want:  "SELECT $$a?b$$",
		},
		{
			name:  "dollar sign positional param untouched",
			query: "SELECT $1, ?",
			want:  "SELECT $1, $1",
		},
		{
			name:  "question mark in line comment ignored",
			query: "SELECT 1 -- why ?\n, ?",
			want:  "SELECT 1 -- why ?\n, $1",
		},
		{
			name:  "question mark in block comment ignored",
			query: "SELECT /* why ? */ ?",
			want:  "SELECT /* why ? */ $1",
		},
		{
			name:  "question mark in nested block comment ignored",
			query: "SELECT /* outer /* ? */ done ? */ ?",
			want:  "SELECT /* outer /* ? */ done ? */ $1",
		},
		{
			name:  "no placeholders",
			query: "SELECT 1",
			want:  "SELECT 1",
		},
		{
			name:  "empty query",
			query: "",
			want:  "",
		},
		{
			name:  "E-string with backslash escaped quote keeps question mark",
			query: `SELECT E'a\'?b' AS s, ?`,
			want:  `SELECT E'a\'?b' AS s, $1`,
		},
		{
			name:  "lowercase e-string with backslash escape keeps question mark",
			query: `SELECT e'line\n?', ?`,
			want:  `SELECT e'line\n?', $1`,
		},
		{
			name:  "E-string escaped quote does not close string before backslash quote",
			query: `SELECT E'it\'s a ?' , E'x?', ?`,
			want:  `SELECT E'it\'s a ?' , E'x?', $1`,
		},
		{
			name:  "jsonb exists operator untouched",
			query: `SELECT data ? 'k'`,
			want:  `SELECT data ? 'k'`,
		},
		{
			name:  "jsonb exists operator adjacent quote",
			query: `SELECT data ?'k'`,
			want:  `SELECT data ?'k'`,
		},
		{
			name:  "jsonb any-key operator untouched",
			query: `SELECT data ?| ARRAY['a', 'b']`,
			want:  `SELECT data ?| ARRAY['a', 'b']`,
		},
		{
			name:  "jsonb all-key operator untouched",
			query: `SELECT data ?& ARRAY['a', 'b']`,
			want:  `SELECT data ?& ARRAY['a', 'b']`,
		},
		{
			name:  "jsonb operators mixed with placeholder",
			query: `SELECT data ? 'k', data ?| ARRAY['x'], data ?& ARRAY['y'] FROM t WHERE id = ?`,
			want:  `SELECT data ? 'k', data ?| ARRAY['x'], data ?& ARRAY['y'] FROM t WHERE id = $1`,
		},
		{
			name:  "placeholder before jsonb operator keeps order",
			query: `SELECT * FROM t WHERE id = ? AND data ? 'k'`,
			want:  `SELECT * FROM t WHERE id = $1 AND data ? 'k'`,
		},
		{
			name:  "jsonb operator then placeholder inside parens",
			query: `SELECT id FROM t WHERE (data ? 'k') AND x = ?`,
			want:  `SELECT id FROM t WHERE (data ? 'k') AND x = $1`,
		},
		{
			name:  "placeholder followed by non-keyword string",
			query: `SELECT * FROM t WHERE a = ? || 'x'`,
			want:  `SELECT * FROM t WHERE a = $1 || 'x'`,
		},
		{
			name:  "placeholder followed by adjacent concat operator",
			query: `SELECT * FROM t WHERE a = ?|| 'x'`,
			want:  `SELECT * FROM t WHERE a = $1|| 'x'`,
		},
		{
			name:  "placeholder followed by adjacent bitwise and",
			query: `SELECT x FROM t WHERE x = ?& m`,
			want:  `SELECT x FROM t WHERE x = ?& m`,
		},
		{
			name:  "jsonb any-key operator adjacent",
			query: `SELECT data ?| ARRAY['a']`,
			want:  `SELECT data ?| ARRAY['a']`,
		},
		{
			name:  "jsonb exists with bound placeholder",
			query: `SELECT data ? ?`,
			want:  `SELECT data ? $1`,
		},
		{
			name:  "jsonb exists with parenthesized bound param",
			query: `SELECT data ? ($1)`,
			want:  `SELECT data ? ($1)`,
		},
		{
			name:  "jsonb exists with parenthesized placeholder",
			query: `SELECT data ? (?)`,
			want:  `SELECT data ? ($1)`,
		},
		{
			name:  "jsonb exists quoted key",
			query: `SELECT id FROM t WHERE data ? 'online'`,
			want:  `SELECT id FROM t WHERE data ? 'online'`,
		},
		{
			name:  "jsonb exists mixed with placeholders",
			query: `SELECT * FROM t WHERE data ? 'k' AND data ? ? AND id = ?`,
			want:  `SELECT * FROM t WHERE data ? 'k' AND data ? $1 AND id = $2`,
		},
		{
			name:  "jsonb operators with arrays and placeholders",
			query: `SELECT data ?| ARRAY['a'], data ?& ARRAY['b'], id = ?`,
			want:  `SELECT data ?| ARRAY['a'], data ?& ARRAY['b'], id = $1`,
		},
		{
			name:  "jsonb exists identifier operand",
			query: `SELECT * FROM t WHERE payload ? key`,
			want:  `SELECT * FROM t WHERE payload ? key`,
		},
		{
			name:  "jsonb exists identifier operand adjacent",
			query: `SELECT * FROM t WHERE payload?key`,
			want:  `SELECT * FROM t WHERE payload?key`,
		},
		{
			name:  "jsonb exists identifier then placeholder",
			query: `SELECT * FROM t WHERE data ? k AND id = ?`,
			want:  `SELECT * FROM t WHERE data ? k AND id = $1`,
		},
		{
			name:  "jsonb exists identifier at end of query",
			query: `SELECT count(*) FROM t WHERE data ? key`,
			want:  `SELECT count(*) FROM t WHERE data ? key`,
		},
		{
			name:  "jsonb exists identifier followed by close paren",
			query: `SELECT * FROM t WHERE (payload ? key)`,
			want:  `SELECT * FROM t WHERE (payload ? key)`,
		},
		{
			name:  "jsonb exists identifier in parens with placeholder",
			query: `SELECT id FROM t WHERE (payload ? key) AND x = ?`,
			want:  `SELECT id FROM t WHERE (payload ? key) AND x = $1`,
		},
		{
			name:  "jsonb exists identifier followed by cast",
			query: `SELECT * FROM t WHERE payload ? key::text`,
			want:  `SELECT * FROM t WHERE payload ? key::text`,
		},
		{
			name:  "jsonb exists identifier followed by comma",
			query: `SELECT payload ? key, id FROM t`,
			want:  `SELECT payload ? key, id FROM t`,
		},
		{
			name:  "jsonb exists identifier followed by close bracket",
			query: `SELECT * FROM t WHERE payload ? key]`,
			want:  `SELECT * FROM t WHERE payload ? key]`,
		},
		{
			name:  "jsonb exists identifier followed by semicolon",
			query: `SELECT * FROM t WHERE payload ? key;`,
			want:  `SELECT * FROM t WHERE payload ? key;`,
		},
		{
			name:  "jsonb exists identifier followed by arrow operator",
			query: `SELECT * FROM t WHERE payload ? key->>'x'`,
			want:  `SELECT * FROM t WHERE payload ? key->>'x'`,
		},
		{
			name:  "placeholder followed by LIMIT keyword",
			query: `SELECT * FROM t WHERE a = ? LIMIT 1`,
			want:  `SELECT * FROM t WHERE a = $1 LIMIT 1`,
		},
		{
			name:  "placeholder followed by AND keyword",
			query: `SELECT * FROM t WHERE x = ? AND y = 2`,
			want:  `SELECT * FROM t WHERE x = $1 AND y = 2`,
		},
		{
			name:  "placeholder followed by lowercase ORDER keyword",
			query: `SELECT * FROM t WHERE a = ? order by x`,
			want:  `SELECT * FROM t WHERE a = $1 order by x`,
		},
		{
			name:  "placeholder followed by FETCH OFFSET keywords",
			query: `SELECT * FROM t WHERE a = ? OFFSET 10 FETCH FIRST 5 ROWS ONLY`,
			want:  `SELECT * FROM t WHERE a = $1 OFFSET 10 FETCH FIRST 5 ROWS ONLY`,
		},
		{
			name:  "escaped double question mark",
			query: `SELECT * FROM t WHERE a = ??`,
			want:  `SELECT * FROM t WHERE a = ?`,
		},
		{
			name:  "doubled quote escape in identifier keeps question mark",
			query: `SELECT "a""?" FROM t WHERE id = ?`,
			want:  `SELECT "a""?" FROM t WHERE id = $1`,
		},
		{
			name:  "jsonb-like ident followed by dot is placeholder",
			query: `SELECT * FROM t WHERE payload ?key.foo`,
			want:  `SELECT * FROM t WHERE payload $1key.foo`,
		},
		{
			name:  "lone dollar at end untouched",
			query: `SELECT $`,
			want:  `SELECT $`,
		},
		{
			name:  "dollar followed by space untouched",
			query: `SELECT $ , ?`,
			want:  `SELECT $ , $1`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := replacePlaceholders(tt.query)
			if got != tt.want {
				t.Errorf("replacePlaceholders(%q) = %q, want %q", tt.query, got, tt.want)
			}
		})
	}
}

func TestReplacePlaceholdersCacheHitSameResult(t *testing.T) {
	t.Parallel()

	query := "SELECT * FROM t WHERE a = ? AND b IN (?, ?, ?) AND c = ?"

	want := replacePlaceholdersUncached(query)

	for i := 0; i < 3; i++ {
		got := replacePlaceholders(query)
		if got != want {
			t.Fatalf("call %d: replacePlaceholders(%q) = %q, want %q", i, query, got, want)
		}
	}
}

func TestReplacePlaceholdersCacheDiscriminates(t *testing.T) {
	t.Parallel()

	q1 := "SELECT * FROM t WHERE a = ?"
	q2 := "SELECT * FROM t WHERE a = ? AND b = ?"

	got1 := replacePlaceholders(q1)
	got2 := replacePlaceholders(q2)

	want1 := "SELECT * FROM t WHERE a = $1"
	want2 := "SELECT * FROM t WHERE a = $1 AND b = $2"

	if got1 != want1 {
		t.Fatalf("q1 = %q, want %q", got1, want1)
	}

	if got2 != want2 {
		t.Fatalf("q2 = %q, want %q", got2, want2)
	}
}

func TestReplacePlaceholdersCacheBounded(t *testing.T) {
	t.Parallel()

	for i := 0; i < placeholderCacheCapacity*4; i++ {
		q := fmt.Sprintf("SELECT * FROM t WHERE a = ? AND id = %d", i)
		_ = replacePlaceholders(q)

		placeholderCacheMu.Lock()
		n := len(placeholderCacheMap)
		placeholderCacheMu.Unlock()

		if n > placeholderCacheCapacity {
			t.Fatalf("after %d distinct queries: cache size = %d, want <= %d", i+1, n, placeholderCacheCapacity)
		}
	}
}

func TestReplacePlaceholdersCacheEvictionStillCorrect(t *testing.T) {
	t.Parallel()

	q := "SELECT * FROM t WHERE evict_probe = ?"
	want := replacePlaceholdersUncached(q)

	for i := 0; i < placeholderCacheCapacity*2; i++ {
		_ = replacePlaceholders(fmt.Sprintf("SELECT * FROM t WHERE filler = ? AND id = %d", i))
	}

	if got := replacePlaceholders(q); got != want {
		t.Fatalf("after eviction: replacePlaceholders(%q) = %q, want %q", q, got, want)
	}
}

func TestReplacePlaceholdersConcurrent(t *testing.T) {
	t.Parallel()

	queries := []string{
		"SELECT * FROM a WHERE x = ?",
		"SELECT * FROM b WHERE y = ? AND z = ?",
		"INSERT INTO c (v) VALUES (?)",
		"UPDATE d SET v = ? WHERE id = ?",
	}

	var wg sync.WaitGroup

	for g := 0; g < 50; g++ {
		wg.Add(1)

		go func(g int) {
			defer wg.Done()

			q := queries[g%len(queries)]

			for i := 0; i < 20; i++ {
				_ = replacePlaceholders(q)
			}
		}(g)
	}

	wg.Wait()
}
