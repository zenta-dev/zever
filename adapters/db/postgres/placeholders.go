package postgres

import (
	"strconv"
	"strings"
	"sync"
)

// placeholderCacheCapacity bounds placeholderCacheMap: key cardinality is the
// number of distinct query texts issued (dozens to low hundreds), so 256
// leaves ample headroom while keeping the map small.
const placeholderCacheCapacity = 256

// placeholderCacheMap memoizes replacePlaceholders output keyed by exact
// input SQL text. replacePlaceholders is pure in that string, so caching by
// input text is safe and turns per-call re-tokenization into one map lookup
// for repeated query texts. Fixed capacity with clear-on-overflow keeps the
// cache bounded under dynamic-SQL churn while staying simple: reads vastly
// outnumber writes in practice.
var (
	placeholderCacheMu  sync.Mutex
	placeholderCacheMap = make(map[string]string, placeholderCacheCapacity)
)

// replacePlaceholders rewrites ? placeholders to $n positional parameters,
// leaving JSONB operators, strings, identifiers, dollar-quoted bodies and
// comments untouched. Results memoize in a bounded mutex-guarded map.
func replacePlaceholders(query string) string {
	placeholderCacheMu.Lock()
	cached, ok := placeholderCacheMap[query]
	placeholderCacheMu.Unlock()

	if ok {
		return cached
	}

	rewritten := replacePlaceholdersUncached(query)

	placeholderCacheMu.Lock()
	if len(placeholderCacheMap) >= placeholderCacheCapacity {
		placeholderCacheMap = make(map[string]string, placeholderCacheCapacity)
	}

	placeholderCacheMap[query] = rewritten
	placeholderCacheMu.Unlock()

	return rewritten
}

// replacePlaceholdersUncached is the single-scan state-machine tokenizer
// behind replacePlaceholders: strings (incl. E-string escapes), quoted idents,
// dollar-quoted bodies, line/block (nestable) comments, ?? escapes, JSONB
// ?/?|/?& operators, and bare ? placeholders numbered left to right.
func replacePlaceholdersUncached(query string) string {
	var sb strings.Builder

	sb.Grow(len(query))

	n := 0
	inString := false
	eString := false
	inIdent := false
	inDollar := false
	inLineComment := false
	blockDepth := 0
	dollarTag := ""

	for i := 0; i < len(query); i++ {
		c := query[i]

		switch {
		case inString:
			if c == '\\' && eString {
				sb.WriteByte(c)

				if i+1 < len(query) {
					sb.WriteByte(query[i+1])

					i++
				}
			} else {
				sb.WriteByte(c)

				if c == '\'' {
					if i+1 < len(query) && query[i+1] == '\'' {
						sb.WriteByte('\'')

						i++
					} else {
						inString = false
						eString = false
					}
				}
			}
		case inIdent:
			sb.WriteByte(c)

			if c == '"' {
				if i+1 < len(query) && query[i+1] == '"' {
					sb.WriteByte('"')

					i++
				} else {
					inIdent = false
				}
			}
		case inDollar:
			if c == '$' && strings.HasPrefix(query[i:], "$"+dollarTag+"$") {
				end := "$" + dollarTag + "$"
				sb.WriteString(end)
				i += len(end) - 1
				inDollar = false
			} else {
				sb.WriteByte(c)
			}
		case inLineComment:
			sb.WriteByte(c)

			if c == '\n' {
				inLineComment = false
			}
		case blockDepth > 0:
			switch {
			case c == '/' && i+1 < len(query) && query[i+1] == '*':
				sb.WriteString("/*")

				blockDepth++
				i++
			case c == '*' && i+1 < len(query) && query[i+1] == '/':
				sb.WriteString("*/")

				blockDepth--
				i++
			default:
				sb.WriteByte(c)
			}
		case c == '\'':
			inString = true
			eString = i >= 1 && (query[i-1] == 'e' || query[i-1] == 'E')

			sb.WriteByte(c)
		case c == '"':
			inIdent = true

			sb.WriteByte(c)
		case c == '-' && i+1 < len(query) && query[i+1] == '-':
			sb.WriteString("--")

			i++
			inLineComment = true
		case c == '/' && i+1 < len(query) && query[i+1] == '*':
			sb.WriteString("/*")

			i++
			blockDepth++
		case c == '$':
			if tag, ok := dollarQuoteStart(query, i); ok {
				sb.WriteString(tag)
				i += len(tag) - 1
				inDollar = true
				dollarTag = tag[1 : len(tag)-1]
			} else {
				sb.WriteByte(c)
			}
		case c == '?':
			switch {
			case i+1 < len(query) && query[i+1] == '?':
				sb.WriteByte('?')

				i++
			case jsonbOperatorAt(query, i):
				sb.WriteByte(c)
			default:
				n++

				sb.WriteString("$")
				sb.WriteString(strconv.Itoa(n))
			}
		default:
			sb.WriteByte(c)
		}
	}

	return sb.String()
}

// jsonbOperatorAt reports whether the ? at index i is a JSONB existence
// (?), any-key (?|) or all-key (?&) operator rather than a placeholder.
func jsonbOperatorAt(query string, i int) bool {
	if i+1 < len(query) && (query[i+1] == '|' || query[i+1] == '&') {
		return i+2 >= len(query) || (query[i+2] != '|' && query[i+2] != '&')
	}

	j := i + 1
	for j < len(query) && isSpace(query[j]) {
		j++
	}

	if j >= len(query) {
		return false
	}

	switch query[j] {
	case '\'', '(', '$', '?', '[', '{':
		return true
	}

	if !isIdentByte(query[j]) {
		return false
	}

	k := j
	for k < len(query) && isIdentByte(query[k]) {
		k++
	}

	if k < len(query) && !isSpace(query[k]) && !isJsonbTerminator(query[k]) {
		return false
	}

	return !isSQLKeyword(query[j:k])
}

// isJsonbTerminator reports bytes that can end a bare JSONB key operand.
func isJsonbTerminator(c byte) bool {
	switch c {
	case ')', ']', ',', ';', ':',
		'+', '-', '*', '/', '<', '>', '=', '~', '!', '@', '#', '%', '^', '&', '|', '?':
		return true
	}

	return false
}

// isSpace reports ASCII whitespace.
func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f'
}

// isSQLKeyword reports idents that end a placeholder (not a JSONB key).
func isSQLKeyword(ident string) bool {
	switch strings.ToUpper(ident) {
	case "AND", "AS", "EXCEPT", "FETCH", "FROM", "GROUP", "HAVING", "IN",
		"INTERSECT", "INTO", "JOIN", "LIMIT", "OFFSET", "ON", "OR", "ORDER",
		"RETURNING", "SET", "UNION", "VALUES", "WHERE", "WINDOW":
		return true
	}

	return false
}

// dollarQuoteStart matches a PostgreSQL dollar-quote opener ($tag$ / $$)
// at index i, returning the full tag on success.
func dollarQuoteStart(query string, i int) (string, bool) {
	if i+1 >= len(query) {
		return "", false
	}

	if query[i+1] == '$' {
		return "$$", true
	}

	if !isIdentByte(query[i+1]) {
		return "", false
	}

	j := i + 1
	for j < len(query) && isIdentByte(query[j]) {
		j++
	}

	if j < len(query) && query[j] == '$' {
		return query[i : j+1], true
	}

	return "", false
}

// isIdentByte reports bytes valid inside a dollar-quote tag / bare ident.
func isIdentByte(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}
