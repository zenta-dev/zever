// Package render (this file) extends the package with JSON operator
// predicate rendering. An NJSON node's base column (Table/Column) is
// rendered as a per-dialect JSON expression -- jsonb operators on Postgres,
// json1 functions on SQLite -- then the comparison (Eq/Neq/Gt/.../In/IsNull)
// is applied against the bound value, exactly the same opSymbol/placeholder
// plumbing renderBinary uses. See render.go's KindJSON.
package render

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/zenta-dev/zever/orm/dialect"
)

// renderJSON renders one NJSON predicate node. n.JSON describes the JSON
// operator and path; n.Op/n.Value are the comparison applied to the
// expression's result (extraction-style operators) or the operator's own
// operand (JSONContains/JSONKeyExists/JSONArrayContains).
//
// The rendering is per-dialect: the node carries an abstract operator, not
// a dialect's syntax. The postgres/sqlite syntax families genuinely differ
// (jsonb operators vs json1 functions), so this is one of the few places
// render branches on dialect identity; an unsupported dialect renders the
// postgres operator form, which a database lacking those operators rejects
// loudly rather than silently returning wrong rows.
//
//nolint:gocyclo // one branch per JSON operator family is inherent to per-dialect rendering
func renderJSON(d dialect.Dialect, n Node, counter *argCounter) (clause string, args []any, err error) {
	if n.JSON == nil {
		return "", nil, nil
	}

	switch n.JSON.Op {
	case JSONExtract, JSONExtractText, JSONPathExtract, JSONPathExtractText:
		clause, args := jsonCompare(d, jsonExprText(d, n), n, counter)

		return clause, args, nil
	case JSONType:
		clause, args := jsonCompare(d, jsonTypeText(d, n), n, counter)

		return clause, args, nil
	case JSONLength:
		// JSONLength renders as a JSON_length function over the column or a
		// nested extraction; the comparison applies against the bound
		// integer.
		clause, args := jsonCompare(d, jsonLengthText(d, n), n, counter)

		return clause, args, nil
	case JSONPathExists, JSONPathMatch:
		// @? / @@ are Postgres-only jsonpath operators. The jsonpath
		// expression is always a BOUND argument (never inlined), so a
		// caller-supplied path can never alter the SQL structure; the
		// optional Steps extract a nested jsonb value to test against.
		if !isPostgres(d) {
			return "", nil, fmt.Errorf("orm/render: %w: dialect %q has no jsonpath %s operator", dialect.ErrUnsupportedByDialect, d.Name(), jsonPathOpName(n.JSON.Op))
		}

		op := "@?"
		if n.JSON.Op == JSONPathMatch {
			op = "@@"
		}

		ph := d.Placeholder(counter.next())

		return jsonExprText(d, n) + " " + op + " " + ph, []any{pathString(n)}, nil
	case JSONPathExistsFunc, JSONPathMatchFunc:
		// The function forms of the same two tests: jsonb_path_exists /
		// jsonb_path_match. Postgres-only (gated above); the path is bound.
		if !isPostgres(d) {
			return "", nil, fmt.Errorf("orm/render: %w: dialect %q has no jsonpath %s operator", dialect.ErrUnsupportedByDialect, d.Name(), jsonPathOpName(n.JSON.Op))
		}

		fn := "jsonb_path_exists"
		if n.JSON.Op == JSONPathMatchFunc {
			fn = "jsonb_path_match"
		}

		ph := d.Placeholder(counter.next())

		return fn + "(" + jsonExprText(d, n) + ", " + ph + ")", []any{pathString(n)}, nil
	case JSONPathQueryFirst:
		// jsonb_path_query_first returns the first jsonb item the jsonpath
		// matches, so it is a jsonb-valued expression comparable against a
		// document exactly like a `->` extraction -- hence the jsonCompare
		// reuse. Postgres-only; the path is bound BEFORE the comparison
		// value, so the placeholder numbering stays clause-ordered.
		if !isPostgres(d) {
			return "", nil, fmt.Errorf("orm/render: %w: dialect %q has no jsonpath %s operator", dialect.ErrUnsupportedByDialect, d.Name(), jsonPathOpName(n.JSON.Op))
		}

		ph := d.Placeholder(counter.next())
		lhs := "jsonb_path_query_first(" + jsonExprText(d, n) + ", " + ph + ")"

		clause, cmpArgs := jsonCompare(d, lhs, n, counter)

		return clause, append([]any{pathString(n)}, cmpArgs...), nil
	case JSONKeyExistsAny, JSONKeyExistsAll:
		// `?|` (any) / `?&` (all): does the jsonb value have any/all of the
		// named keys? Postgres-only. Each key is bound individually and
		// assembled into an ARRAY[...] text[] operand, so a caller-supplied
		// key can never inject SQL. An empty key list is logically vacuous:
		// `any of {}` is false, `all of {}` is true -- rendered as the
		// constant truth value rather than an unintuitive ARRAY[] operand.
		if !isPostgres(d) {
			return "", nil, fmt.Errorf("orm/render: %w: dialect %q has no jsonpath %s operator", dialect.ErrUnsupportedByDialect, d.Name(), jsonPathOpName(n.JSON.Op))
		}

		keys := keysOf(n)
		if len(keys) == 0 {
			if n.JSON.Op == JSONKeyExistsAll {
				return "1 = 1", nil, nil
			}

			return "1 = 0", nil, nil
		}

		op := "?|"
		if n.JSON.Op == JSONKeyExistsAll {
			op = "?&"
		}

		phs := make([]string, len(keys))
		for i := range keys {
			phs[i] = d.Placeholder(counter.next())
		}

		return jsonExprText(d, n) + " " + op + " ARRAY[" + strings.Join(phs, ", ") + "]", keysAny(keys), nil
	case JSONContains:
		// @> is a jsonb containment operator; only the postgres builder
		// produces JSONContains nodes, so this always renders the postgres
		// form. Executing such a predicate on a database without the
		// operator (e.g. sqlite) is a caller error the DB rejects loudly,
		// never a silent wrong result.
		ph := d.Placeholder(counter.next())

		return jsonExprText(d, n) + " @> " + ph + "::jsonb", []any{n.Value}, nil
	case JSONKeyExists:
		if isSQLite(d) {
			// json1 has no `?` operator; the idiomatic existence test is
			// json_extract(col, '$.key') IS NOT NULL. The extraction path
			// is the node's own steps plus the key (n.Value) as the final
			// step.
			steps := append(append([]JSONStep(nil), n.JSON.Steps...), JSONStep{Key: keyOf(n)})

			return jsonExtract(d, n, steps) + " IS NOT NULL", nil, nil
		}

		ph := d.Placeholder(counter.next())

		return jsonExprText(d, n) + " ? " + ph, []any{keyOf(n)}, nil
	case JSONArrayContains:
		// sqlite json1: does the JSON array (at the optional path) contain
		// this element? Rendered as an EXISTS over the json_each
		// table-valued function, whose `value` column holds each element.
		ph := d.Placeholder(counter.next())

		var b strings.Builder

		b.WriteString("EXISTS (SELECT 1 FROM json_each(")
		b.WriteString(quoteColumn(d, n.Column))

		if len(n.JSON.Steps) > 0 {
			b.WriteString(", ")
			b.WriteString(sqlitePathLiteral(n.JSON.Steps))
		}

		b.WriteString(") WHERE value = ")
		b.WriteString(ph)
		b.WriteString(")")

		return b.String(), []any{n.Value}, nil
	default:
		return "", nil, fmt.Errorf("orm/render: unsupported JSON operator %d", n.JSON.Op)
	}
}

// jsonCompare applies the node's comparison (n.Op) to a JSON expression's
// result (lhs): `lhs <op> <placeholder>`, `lhs IS [NOT] NULL`, or
// `lhs IN (...)`. Mirrors renderBinary's operator/placeholder plumbing. A
// jsonb-valued extraction compared with a bound value on postgres gets an
// explicit `::jsonb` cast on the placeholder (`jsonb = $1::jsonb`), since
// postgres has no implicit text→jsonb coercion in that position. An
// inapplicable comparison (LIKE, BETWEEN, NOT IN, or any other operator the
// JSON builders never produce) renders no clause rather than failing: the
// caller could not have built it, so there is nothing to fail closed on.
func jsonCompare(d dialect.Dialect, lhs string, n Node, counter *argCounter) (clause string, args []any) {
	switch n.Op { //nolint:exhaustive // only the JSON comparison ops are legal; the rest fall through to no clause
	case OpIsNull:
		return lhs + " IS NULL", nil
	case OpIsNotNull:
		return lhs + " IS NOT NULL", nil
	case OpIn:
		vs, _ := n.Value.([]any)

		if len(vs) == 0 {
			return "1 = 0", nil
		}

		phs := make([]string, len(vs))
		for i := range vs {
			phs[i] = d.Placeholder(counter.next()) + jsonCast(d, n.JSON.Op)
		}

		return lhs + " IN (" + strings.Join(phs, ", ") + ")", vs
	case OpEq, OpNeq, OpGt, OpGte, OpLt, OpLte:
		// The case list pre-validates the operator, so opSymbol cannot
		// fail here; any other Op falls through to no clause below.
		symbol, _ := opSymbol(n.Op)

		ph := d.Placeholder(counter.next()) + jsonCast(d, n.JSON.Op)

		return lhs + " " + symbol + " " + ph, []any{n.Value}
	case OpLike, OpBetween, OpNotIn:
		// A JSON expression compared with LIKE/BETWEEN -- or a JSON NOT IN
		// -- is not produced by the JSON builders in this phase (only the
		// comparison set above is exposed); fall through to nothing rather
		// than render a clause the caller could not have built.
		return "", nil
	}

	return "", nil
}

// jsonCast returns the `::jsonb` cast suffix for a jsonb-valued extraction
// on postgres (where the comparison target must be jsonb, not text); empty
// on sqlite (which has no such cast syntax -- its JSON functions already
// return typed JSON values) and for text-valued extractions
// (the arrow-text operators, json_extract).
func jsonCast(d dialect.Dialect, op JSONOp) string {
	if isSQLite(d) {
		return ""
	}

	if op == JSONExtract || op == JSONPathExtract || op == JSONPathQueryFirst {
		return "::jsonb"
	}

	return ""
}

// jsonExprText renders the JSON extraction expression for a node's base
// column and path steps: jsonb operators on postgres (`->`, `->>`, `#>`,
// `#>>`), a single json_extract call on sqlite. JSONContains and
// JSONKeyExists reuse this for their LHS -- the base column plus the
// extracted path -- and render their own operator afterwards.
func jsonExprText(d dialect.Dialect, n Node) string {
	if isSQLite(d) {
		return jsonExtract(d, n, n.JSON.Steps)
	}

	col := quoteColumn(d, n.Column)

	switch n.JSON.Op { //nolint:exhaustive // JSONType/JSONArrayContains/JSONLength are rendered by renderJSON before ever reaching this helper
	case JSONExtract, JSONExtractText, JSONContains, JSONKeyExists,
		JSONPathExists, JSONPathMatch, JSONPathExistsFunc, JSONPathMatchFunc,
		JSONPathQueryFirst, JSONKeyExistsAny, JSONKeyExistsAll:
		return pgArrowChain(col, n.JSON.Steps, n.JSON.Op == JSONExtractText)
	case JSONPathExtract, JSONPathExtractText:
		op := "#>"
		if n.JSON.Op == JSONPathExtractText {
			op = "#>>"
		}

		return col + op + pgPathArray(n.JSON.Steps)
	default:
		return col
	}
}

// jsonLengthText renders a JSON_LENGTH expression: JSON_LENGTH(col) for the
// whole document, or JSON_LENGTH(JSON_EXTRACT(col, '$.path')) for a nested
// value.
func jsonLengthText(d dialect.Dialect, n Node) string {
	col := quoteColumn(d, n.Column)

	if len(n.JSON.Steps) == 0 {
		return "JSON_LENGTH(" + col + ")"
	}

	return "JSON_LENGTH(JSON_EXTRACT(" + col + sqlitePathArg(n.JSON.Steps) + "))"
}

// jsonTypeText renders a value-type expression: jsonb_typeof(col->'a') on
// postgres, json_type(col, '$.a') on sqlite.
func jsonTypeText(d dialect.Dialect, n Node) string {
	if isSQLite(d) {
		return "json_type(" + quoteColumn(d, n.Column) + sqlitePathArg(n.JSON.Steps) + ")"
	}

	return "jsonb_typeof(" + pgArrowChain(quoteColumn(d, n.Column), n.JSON.Steps, false) + ")"
}

// jsonExtract renders a sqlite json1 json_extract call over col, with the
// path steps as a '$'-rooted json path argument (or no path at all when
// steps is empty, which extracts the whole document).
func jsonExtract(d dialect.Dialect, n Node, steps []JSONStep) string {
	return "json_extract(" + quoteColumn(d, n.Column) + sqlitePathArg(steps) + ")"
}

// isSQLite reports whether d is the sqlite dialect, whose JSON rendering
// uses the json1 function family (json_extract/json_type/json_each) rather
// than jsonb operators.
func isSQLite(d dialect.Dialect) bool { return d.Name() == "sqlite" }

// isPostgres reports whether d is the postgres dialect. The JSON path
// operators (`@?`, `@@`, `?|`, `?&`, the jsonb_path_* functions) exist only
// on Postgres; renderJSON gates them here and returns a typed
// dialect.ErrUnsupportedByDialect on every other dialect.
func isPostgres(d dialect.Dialect) bool { return d.Name() == "postgres" }

// pathString returns the bound jsonpath expression a JSONPathExists/...
// node carries in its JSON.Path.
func pathString(n Node) string {
	return n.JSON.Path
}

// keysOf returns the key list a JSONKeyExistsAny/All node carries in its
// Value (always a []string, as built by the postgres package).
func keysOf(n Node) []string {
	ks, _ := n.Value.([]string)

	return ks
}

// keysAny converts a []string key list to the []any args slice the renderer
// returns.
func keysAny(ks []string) []any {
	out := make([]any, len(ks))
	for i, k := range ks {
		out[i] = k
	}

	return out
}

// jsonPathOpName returns a human-readable name for the postgres-only
// jsonpath operators, for the typed unsupported-dialect error.
func jsonPathOpName(op JSONOp) string {
	switch op { //nolint:exhaustive // only the postgres-only jsonpath ops are named; every other op falls through
	case JSONPathExists:
		return "@?"
	case JSONPathMatch:
		return "@@"
	case JSONPathExistsFunc:
		return "jsonb_path_exists"
	case JSONPathMatchFunc:
		return "jsonb_path_match"
	case JSONPathQueryFirst:
		return "jsonb_path_query_first"
	case JSONKeyExistsAny:
		return "?|"
	case JSONKeyExistsAll:
		return "?&"
	default:
		return "JSON"
	}
}

// keyOf returns the JSON key a JSONKeyExists node tests for. The key is the
// node's single value (the the JSON builders set both n.Value and the key
// path step for sqlite's json_extract rendering).
func keyOf(n Node) string {
	s, _ := n.Value.(string)

	return s
}

// pgArrowChain renders a postgres jsonb operator chain: `col->'a'->0`, or
// with text=true the text-extraction form `col->'a'->>'b'` (only the final
// step uses ->>; earlier steps must stay `->` since ->> yields text, not
// jsonb).
func pgArrowChain(col string, steps []JSONStep, text bool) string {
	var b strings.Builder

	b.WriteString(col)

	for i, s := range steps {
		if text && i == len(steps)-1 {
			b.WriteString("->>")
		} else {
			b.WriteString("->")
		}

		if s.IsIndex {
			b.WriteString(strconv.FormatInt(s.Index, 10))

			continue
		}

		b.WriteString(sqlStringLiteral(s.Key))
	}

	return b.String()
}

// pgPathArray renders a postgres text[] array literal for the #>/#>> path
// operators, e.g. `'{"a","b"}'`. Every element is quoted to avoid postgres
// array-literal parsing surprises; index steps render as bare integers
// (postgres jsonb path operators treat integer-looking elements as array
// indexes).
func pgPathArray(steps []JSONStep) string {
	parts := make([]string, len(steps))

	for i, s := range steps {
		if s.IsIndex {
			parts[i] = strconv.FormatInt(s.Index, 10)

			continue
		}

		parts[i] = `"` + pgArrayEscape(s.Key) + `"`
	}

	return "'{" + strings.Join(parts, ",") + "}'"
}

// pgArrayEscape escapes a key for use inside a double-quoted postgres
// array-literal element: `"` and `\` are backslash-escaped, and any `'` is
// doubled for the enclosing single-quoted literal.
func pgArrayEscape(s string) string {
	s = strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)

	return strings.ReplaceAll(s, "'", "''")
}

// sqlitePathLiteral renders a sqlite json1 path as a SQL string literal:
// '$'-rooted, `.key` for simple keys, `$."key"` (double-quoted) for keys
// with special characters, `[N]` for array indexes. Returns the quoted
// literal including surrounding single quotes.
func sqlitePathLiteral(steps []JSONStep) string {
	var b strings.Builder

	b.WriteByte('\'')

	b.WriteString(sqlitePathBody(steps))

	b.WriteByte('\'')

	return b.String()
}

// sqlitePathArg renders sqlite json path steps as a `, '<path>'` argument
// fragment (empty when there are no steps, so json_extract(col) extracts
// the whole document).
func sqlitePathArg(steps []JSONStep) string {
	if len(steps) == 0 {
		return ""
	}

	return ", " + sqlitePathLiteral(steps)
}

// sqlitePathBody renders the unquoted '$'-rooted json path for steps.
func sqlitePathBody(steps []JSONStep) string {
	var b strings.Builder

	b.WriteByte('$')

	for _, s := range steps {
		if s.IsIndex {
			b.WriteByte('[')
			b.WriteString(strconv.FormatInt(s.Index, 10))
			b.WriteByte(']')

			continue
		}

		if isSimpleKey(s.Key) {
			b.WriteByte('.')
			b.WriteString(s.Key)

			continue
		}

		b.WriteString(`."`)
		b.WriteString(strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s.Key))
		b.WriteByte('"')
	}

	return b.String()
}

// isSimpleKey reports whether a JSON key is a plain `[A-Za-z_][A-Za-z0-9_]*`
// identifier that needs no quoting inside a json path.
func isSimpleKey(s string) bool {
	if s == "" {
		return false
	}

	for i, r := range s {
		switch {
		case r == '_':
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}

	return true
}

// sqlStringLiteral wraps s in single quotes, doubling any embedded quote --
// the postgres/SQLite standard-conforming string-literal escaping for the
// key operands of `->`/`->>`.
func sqlStringLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
