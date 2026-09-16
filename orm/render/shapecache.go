package render

import (
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/zenta-dev/zever/orm/dialect"
)

// shapeCacheCapacity bounds the render-shape cache. Key cardinality is the
// number of DISTINCT query shapes a generated application issues (typically
// dozens to low hundreds: one per generated finder/list/mutation method),
// so 256 leaves ample headroom while keeping the map small.
const shapeCacheCapacity = 256

// shapeCache memoizes rendered SQL TEXT keyed by a structural fingerprint
// of a query "shape" -- dialect, table, columns, the predicate tree's
// operator/column structure (never its literal VALUES), order terms, and
// limit/offset PRESENCE -- so a repeated identical shape skips the per-call
// text build. Its bound arguments are still collected fresh every call
// (they vary per call) via the mirrored arg collectors below. This ports
// the legacy orm/engine/shapecache.go idea into orm/render; the byte-
// identical contract is enforced by shapecache_test.go.
//
// It is a fixed-capacity map guarded by a mutex: when an insert would
// exceed capacity the map is cleared and the new entry inserted. That keeps
// the cache bounded under pathological shape churn while staying simple and
// deterministic for tests (reads vastly outnumber writes in practice).
var (
	shapeCacheMu     sync.Mutex
	shapeCacheMap    = make(map[string]string, shapeCacheCapacity)
	shapeCacheHits   atomic.Int64
	shapeCacheMisses atomic.Int64
)

// resetShapeCache empties the cache and zeroes the hit/miss counters, for
// tests to exercise the cache deterministically.
func resetShapeCache() {
	shapeCacheMu.Lock()
	shapeCacheMap = make(map[string]string, shapeCacheCapacity)
	shapeCacheMu.Unlock()

	shapeCacheHits.Store(0)
	shapeCacheMisses.Store(0)
}

// shapeCacheLen reports the number of cached entries, for tests.
func shapeCacheLen() int {
	shapeCacheMu.Lock()
	defer shapeCacheMu.Unlock()

	return len(shapeCacheMap)
}

func shapeCacheGet(key string) (string, bool) {
	shapeCacheMu.Lock()
	q, ok := shapeCacheMap[key]
	shapeCacheMu.Unlock()

	if ok {
		shapeCacheHits.Add(1)
	} else {
		shapeCacheMisses.Add(1)
	}

	return q, ok
}

func shapeCachePut(key, query string) {
	shapeCacheMu.Lock()
	if len(shapeCacheMap) >= shapeCacheCapacity {
		shapeCacheMap = make(map[string]string, shapeCacheCapacity)
	}

	shapeCacheMap[key] = query
	shapeCacheMu.Unlock()
}

// fsep separates fingerprint fields. It is not a character SQL identifiers
// or codegen table/column names can contain, so it cannot be produced by
// user-controlled input and collide two different shapes into the same key.
const fsep = "\x1f"

// cacheableWhere reports whether where's predicate tree renders through the
// shape cache. The cacheable kinds are the plain comparison/composition set
// (Binary/In/Between/Like/Compound); Raw, JSON, FTS, subquery shapes
// (KindSubquery, and In/binary comparisons against a Subquery value), a
// correlated binary whose Value is an OuterRef, and reserved kinds render
// fresh every call -- they are rare, and caching them would expand the
// fingerprint surface (an In node's value shape, a nested subquery's whole
// where/order tree, dialect-version-dependent JSON rendering, raw
// fragments) for no measured gain.
func cacheableWhere(n Node) bool {
	switch n.Kind { //nolint:exhaustive // uncacheable kinds rejected by default
	case KindNone, KindBetween, KindLike:
		return true
	case KindBinary:
		if _, ok := n.Value.(Subquery); ok {
			// A scalar comparison against a subquery (its inner where/order/
			// projection and its arg bound-values would all become part of
			// the text shape) -- render fresh.
			return false
		}

		if _, ok := n.Value.(OuterRef); ok {
			// A correlated binary renders DIFFERENT text depending on the
			// statement's enclosing table name, which is not in the shape
			// fingerprint -- and collectArgs would bind the marker as an
			// argument on a hit. Render fresh so the enclosing table name
			// is resolved at execution time; the operator/column shape
			// collides with an identical plain-value predicate only in
			// SPIRIT, safe to keep uncached forever.
			return false
		}

		return true
	case KindIn:
		if _, ok := n.Value.(Subquery); ok {
			return false
		}

		return true
	case KindCompound:
		for _, c := range n.Children {
			if !cacheableWhere(c) {
				return false
			}
		}

		return true
	default:
		return false
	}
}

// cacheableOrder reports whether every order term renders through the shape
// cache. Plain column terms do; an FTS ranking term or a scalar-expression
// term (Func) does not -- their rendered text depends on an expression tree
// (and binds its own arguments), so they render fresh every call, matching
// cacheableWhere's treatment of Raw/JSON/FTS/subquery shapes.
func cacheableOrder(order []OrderTerm) bool {
	for _, o := range order {
		if o.FTS != nil || o.Func != nil {
			return false
		}
	}

	return true
}

// writeDialectKey writes the dialect parts the renderers branch on: the
// name, the JSON `->>` capability flag (see dialect.JSONDialect), and the
// NULLS ordering capability flag (version-gated on the sqlite dialect), so
// two dialects of the same name with different capabilities never share a
// cache entry. It writes straight into the caller's builder (rather than returning
// a concatenated string) so a per-call shape key builds with no intermediate
// string allocation.
func writeDialectKey(b *strings.Builder, d dialect.Dialect) {
	b.WriteString(d.Name())
	b.WriteString(fsep)

	if jd, ok := d.(dialect.JSONDialect); ok && !jd.SupportsJSONArrowText() {
		b.WriteString("0")
	} else {
		b.WriteString("1")
	}

	b.WriteString(fsep)

	if nd, ok := d.(dialect.NullsOrderDialect); ok && nd.SupportsNullsOrdering() {
		b.WriteString("1")
	} else {
		b.WriteString("0")
	}
}

// writeFingerprintExpr writes the structural signature of a cacheable
// predicate tree: operator kinds and column names, never literal values --
// except Op.In, whose rendered placeholder COUNT depends on the argument
// slice's length, which is therefore part of the shape (a 2-element IN and a
// 3-element IN render different SQL text). It writes straight into the
// caller's builder, matching fingerprintExpr's historical byte output so the
// shape cache's identity is unchanged.
func writeFingerprintExpr(b *strings.Builder, n Node) {
	switch n.Kind { //nolint:exhaustive // uncacheable kinds handled by default
	case KindNone:
		return
	case KindBinary:
		b.WriteString("B")
		b.WriteString(fsep)
		b.WriteString(n.Table)
		b.WriteString(fsep)
		b.WriteString(n.Column)
		b.WriteString(fsep)
		b.WriteString(strconv.Itoa(int(n.Op)))
	case KindIn:
		b.WriteString("I")
		b.WriteString(fsep)
		b.WriteString(n.Table)
		b.WriteString(fsep)
		b.WriteString(n.Column)
		b.WriteString(fsep)
		b.WriteString(strconv.Itoa(inLen(n.Value)))
	case KindBetween:
		b.WriteString("W")
		b.WriteString(fsep)
		b.WriteString(n.Table)
		b.WriteString(fsep)
		b.WriteString(n.Column)
	case KindLike:
		b.WriteString("K")
		b.WriteString(fsep)
		b.WriteString(n.Table)
		b.WriteString(fsep)
		b.WriteString(n.Column)
	case KindCompound:
		b.WriteString("C")
		b.WriteString(fsep)
		b.WriteString(strconv.Itoa(int(n.Compound)))
		b.WriteString(fsep)

		for i, c := range n.Children {
			if i > 0 {
				b.WriteString(fsep)
			}

			writeFingerprintExpr(b, c)
		}
	default:
		// Uncacheable kinds never reach here (cacheableWhere guards the
		// wrappers), but the fingerprint must still be unique against the
		// cacheable ones so a stray call can never collide.
		b.WriteString("X")
		b.WriteString(fsep)
		b.WriteString(strconv.Itoa(int(n.Kind)))
	}
}

// inLen returns the placeholder count an In node renders, mirroring
// renderIn: a non-[]any or empty value renders "1 = 0" (zero placeholders).
func inLen(value any) int {
	if vs, ok := value.([]any); ok {
		return len(vs)
	}

	return 0
}

// writeFingerprintOrder writes the plain-column ORDER BY list fingerprint
// (cacheable terms only -- the wrappers reject FTS terms before this is
// reached). The NULLS position is part of the fingerprint, and the dialect's
// NULLS capability is written by writeDialectKey, so a NULLS-bearing shape
// can never share a cache entry with one that would drop (or must reject)
// the modifier. It writes straight into the caller's builder.
func writeFingerprintOrder(b *strings.Builder, order []OrderTerm) {
	for i, o := range order {
		if i > 0 {
			b.WriteString(fsep)
		}

		b.WriteString(o.Column)
		b.WriteString(fsep)

		if o.Desc {
			b.WriteString("D")
		}

		b.WriteString("N")
		b.WriteString(strconv.Itoa(int(o.Nulls)))
	}
}

// writeJoinedColumns writes cols separated by fsep, avoiding the
// intermediate joined-string allocation strings.Join would make.
func writeJoinedColumns(b *strings.Builder, cols []string) {
	for i, c := range cols {
		if i > 0 {
			b.WriteString(fsep)
		}

		b.WriteString(c)
	}
}

func selectShapeKey(d dialect.Dialect, table string, columns []string, where Node, order []OrderTerm, limit, offset int, mods SelectModifiers) string {
	var b strings.Builder

	b.Grow(128)

	b.WriteString("SEL")
	b.WriteString(fsep)
	writeDialectKey(&b, d)
	b.WriteString(fsep)
	b.WriteString(table)
	b.WriteString(fsep)
	writeJoinedColumns(&b, columns)
	b.WriteString(fsep)
	writeFingerprintExpr(&b, where)
	b.WriteString(fsep)
	writeFingerprintOrder(&b, order)
	b.WriteString(fsep)
	writeFingerprintModifiers(&b, mods)
	b.WriteString(fsep)

	if limit > 0 {
		b.WriteString("L")
	}

	if offset > 0 {
		b.WriteString("O")
	}

	return b.String()
}

// writeFingerprintModifiers fingerprints DISTINCT, DISTINCT ON and the
// row-lock clause so two modifier shapes never share a cache entry that would
// drop the modifier.
func writeFingerprintModifiers(b *strings.Builder, mods SelectModifiers) {
	b.WriteString("M")

	if mods.Distinct {
		b.WriteString("D")
	}

	if mods.DistinctOn != nil {
		b.WriteString("O")
		b.WriteString(fsep)
		writeJoinedColumns(b, mods.DistinctOn)
		b.WriteString(fsep)
	}

	b.WriteString(strconv.Itoa(int(mods.Lock)))

	if mods.LockOf != nil {
		b.WriteString("F")
		b.WriteString(fsep)
		writeJoinedColumns(b, mods.LockOf)
		b.WriteString(fsep)
	}

	if mods.NoWait {
		b.WriteString("N")
	}

	if mods.SkipLocked {
		b.WriteString("K")
	}

	if mods.Tablesample.Method != "" {
		b.WriteString("T")
		b.WriteString(fsep)
		b.WriteString(strings.ToUpper(mods.Tablesample.Method))
		b.WriteString(fsep)
		b.WriteString(strconv.FormatFloat(mods.Tablesample.Arg, 'f', -1, 64))
	}
}

func countShapeKey(d dialect.Dialect, table string, where Node) string {
	var b strings.Builder

	b.Grow(64)

	b.WriteString("CNT")
	b.WriteString(fsep)
	writeDialectKey(&b, d)
	b.WriteString(fsep)
	b.WriteString(table)
	b.WriteString(fsep)
	writeFingerprintExpr(&b, where)

	return b.String()
}

// updateShapeKey fingerprints a cacheable UPDATE shape: table, SET
// columns, the where tree, and the order/limit/offset presence (mirroring
// selectShapeKey's tail, so an ordered or limited UPDATE never shares a
// cache entry with the plain one). FTS order terms are rejected by the
// caller's cacheableOrder guard before this is reached.
func updateShapeKey(d dialect.Dialect, table string, sets []Assignment, where Node, order []OrderTerm, limit, offset int) string {
	var b strings.Builder

	b.Grow(128)

	b.WriteString("UPD")
	b.WriteString(fsep)
	writeDialectKey(&b, d)
	b.WriteString(fsep)
	b.WriteString(table)
	b.WriteString(fsep)

	for i, s := range sets {
		if i > 0 {
			b.WriteString(fsep)
		}

		b.WriteString(s.Column)
	}

	b.WriteString(fsep)
	writeFingerprintExpr(&b, where)
	b.WriteString(fsep)
	writeFingerprintOrder(&b, order)
	b.WriteString(fsep)

	if limit > 0 {
		b.WriteString("L")
	}

	if offset > 0 {
		b.WriteString("O")
	}

	return b.String()
}

// deleteShapeKey fingerprints a cacheable DELETE shape: table, the where
// tree, and order/limit/offset presence.
func deleteShapeKey(d dialect.Dialect, table string, where Node, order []OrderTerm, limit, offset int) string {
	var b strings.Builder

	b.Grow(64)

	b.WriteString("DEL")
	b.WriteString(fsep)
	writeDialectKey(&b, d)
	b.WriteString(fsep)
	b.WriteString(table)
	b.WriteString(fsep)
	writeFingerprintExpr(&b, where)
	b.WriteString(fsep)
	writeFingerprintOrder(&b, order)
	b.WriteString(fsep)

	if limit > 0 {
		b.WriteString("L")
	}

	if offset > 0 {
		b.WriteString("O")
	}

	return b.String()
}

func insertShapeKey(d dialect.Dialect, table string, columns []string, n int) string {
	var b strings.Builder

	b.Grow(64)

	b.WriteString("INS")
	b.WriteString(fsep)
	writeDialectKey(&b, d)
	b.WriteString(fsep)
	b.WriteString(table)
	b.WriteString(fsep)
	writeJoinedColumns(&b, columns)
	b.WriteString(fsep)
	b.WriteString(strconv.Itoa(n))

	return b.String()
}

func insertManyShapeKey(d dialect.Dialect, table string, columns []string, rows [][]any) string {
	var b strings.Builder

	b.Grow(64 + len(columns)*8 + len(rows)*4)

	b.WriteString("INSM")
	b.WriteString(fsep)
	writeDialectKey(&b, d)
	b.WriteString(fsep)
	b.WriteString(table)
	b.WriteString(fsep)
	writeJoinedColumns(&b, columns)
	b.WriteString(fsep)
	b.WriteString(strconv.Itoa(len(rows)))
	b.WriteString(fsep)

	// Per-row widths are part of the shape: InsertMany validates every row
	// against the column count, and a width mismatch is a rendering-time
	// error, so two calls with the same column list and row COUNT but
	// different row WIDTHS must not share a cache entry.
	for _, r := range rows {
		b.WriteString(strconv.Itoa(len(r)))
		b.WriteString(fsep)
	}

	return b.String()
}

// collectArgs walks a cacheable predicate tree collecting only its bound
// VALUES, in exactly the order the cacheable render paths emit their
// placeholders -- the mirror of renderExpr's arg accumulation, used on
// cache HITS so a repeated shape never re-renders text. The byte-identical
// contract with the render path is enforced by the shape-cache tests.
func collectArgs(n Node, out *[]any) {
	switch n.Kind { //nolint:exhaustive // uncacheable kinds never reach here
	case KindNone:
		return
	case KindBinary:
		switch n.Op { //nolint:exhaustive // only the two null ops skip args; the rest bind one
		case OpIsNull, OpIsNotNull:
			return
		default:
			*out = append(*out, n.Value)
		}
	case KindIn:
		if vs, ok := n.Value.([]any); ok {
			*out = append(*out, vs...)
		}
	case KindBetween:
		lo, hi := any(nil), any(nil)
		if pair, ok := n.Value.([2]any); ok {
			lo, hi = pair[0], pair[1]
		}

		*out = append(*out, lo, hi)
	case KindLike:
		*out = append(*out, n.Value)
	case KindCompound:
		if n.Compound == CompoundNot {
			if len(n.Children) == 1 {
				collectArgs(n.Children[0], out)
			}

			return
		}

		for _, c := range n.Children {
			collectArgs(c, out)
		}
	}
}

// countCollectedArgs returns how many values collectArgs would append for
// n, mirroring its Kind handling exactly, so the collectors can allocate the
// exact result length once instead of growing through repeated appends.
func countCollectedArgs(n Node) int {
	switch n.Kind { //nolint:exhaustive // mirrors collectArgs's uncacheable default
	case KindNone:
		return 0
	case KindBinary:
		switch n.Op { //nolint:exhaustive // only the two null ops bind nothing
		case OpIsNull, OpIsNotNull:
			return 0
		default:
			return 1
		}
	case KindIn:
		if vs, ok := n.Value.([]any); ok {
			return len(vs)
		}

		return 0
	case KindBetween:
		return 2
	case KindLike:
		return 1
	case KindCompound:
		if n.Compound == CompoundNot {
			if len(n.Children) == 1 {
				return countCollectedArgs(n.Children[0])
			}

			return 0
		}

		total := 0

		for _, c := range n.Children {
			total += countCollectedArgs(c)
		}

		return total
	default:
		return 0
	}
}

func selectArgs(where Node, _ []OrderTerm, limit, offset int) []any {
	n := countCollectedArgs(where)
	if limit > 0 {
		n++
	}

	if offset > 0 {
		n++
	}

	var args []any
	if n > 0 {
		args = make([]any, 0, n)
	}

	collectArgs(where, &args)

	if limit > 0 {
		args = append(args, limit)
	}

	if offset > 0 {
		args = append(args, offset)
	}

	return args
}

func countArgs(where Node) []any {
	n := countCollectedArgs(where)

	var args []any
	if n > 0 {
		args = make([]any, 0, n)
	}

	collectArgs(where, &args)

	return args
}

// updateArgs copies the bound arguments of a cacheable UPDATE shape: SET
// values, where values, then the limit/offset when present -- the mirror
// of renderUpdateText's placeholder order. Order terms bind nothing unless
// an FTS ranking term carries one, and such shapes are never cached
// (cacheableOrder rejects them), so order is deliberately skipped.
func updateArgs(sets []Assignment, where Node, limit, offset int) []any {
	n := len(sets) + countCollectedArgs(where)
	if limit > 0 {
		n++
	}

	if offset > 0 {
		n++
	}

	var args []any
	if n > 0 {
		args = make([]any, 0, n)
	}

	for _, s := range sets {
		args = append(args, s.Value)
	}

	collectArgs(where, &args)

	if limit > 0 {
		args = append(args, limit)
	}

	if offset > 0 {
		args = append(args, offset)
	}

	return args
}

func deleteArgs(where Node, limit, offset int) []any {
	n := countCollectedArgs(where)
	if limit > 0 {
		n++
	}

	if offset > 0 {
		n++
	}

	var args []any
	if n > 0 {
		args = make([]any, 0, n)
	}

	collectArgs(where, &args)

	if limit > 0 {
		args = append(args, limit)
	}

	if offset > 0 {
		args = append(args, offset)
	}

	return args
}

// insertArgs copies values truncated to n columns, mirroring
// renderInsertText's shorter-side-wins rule: a call with more values than
// columns drops the extras, so the cached hit must too.
func insertArgs(values []any, n int) []any {
	if n == 0 {
		return nil
	}

	args := make([]any, n)
	copy(args, values)

	return args
}

func insertManyArgs(rows [][]any) []any {
	n := 0
	for _, row := range rows {
		n += len(row)
	}

	var args []any
	if n > 0 {
		args = make([]any, 0, n)
	}

	for _, row := range rows {
		args = append(args, row...)
	}

	return args
}

// Select renders a SELECT statement with no DISTINCT/locking modifiers,
// caching the rendered SQL text under its shape fingerprint so a repeated
// identical shape skips the text build (see shapeCache). It is exactly
// SelectMods with the zero SelectModifiers value.
func Select(
	d dialect.Dialect,
	table string,
	columns []string,
	where Node,
	order []OrderTerm,
	limit, offset int,
) (query string, args []any, err error) {
	return SelectMods(d, table, columns, where, order, limit, offset, SelectModifiers{})
}

// SelectMods renders a SELECT statement carrying DISTINCT and/or a
// row-level lock, caching the rendered SQL text under its shape fingerprint
// the same way Select does. DISTINCT and the lock mode (with its optional
// NOWAIT/SKIP LOCKED suffix) are part of the fingerprint, so a modifier
// shape can never hit a cache entry that omits it. Cacheable shapes (plain
// comparison/composition predicates, plain order terms) collect their bound
// arguments via selectArgs on a hit; uncacheable shapes (Raw/JSON/FTS)
// always render fresh. An invalid DISTINCT + lock combination is a
// rendering-time error and is never cached.
func SelectMods(
	d dialect.Dialect,
	table string,
	columns []string,
	where Node,
	order []OrderTerm,
	limit, offset int,
	mods SelectModifiers,
) (query string, args []any, err error) {
	if !cacheableWhere(where) || !cacheableOrder(order) {
		return renderSelectText(d, table, columns, where, order, limit, offset, mods)
	}

	key := selectShapeKey(d, table, columns, where, order, limit, offset, mods)

	if q, ok := shapeCacheGet(key); ok {
		return q, selectArgs(where, order, limit, offset), nil
	}

	q, args, err := renderSelectText(d, table, columns, where, order, limit, offset, mods)
	if err != nil {
		return "", nil, err
	}

	shapeCachePut(key, q)

	return q, args, nil
}

// Count renders a `SELECT COUNT(*)` statement with the same shape-cache
// treatment as Select; see Select's doc comment.
func Count(d dialect.Dialect, table string, where Node) (query string, args []any, err error) {
	if !cacheableWhere(where) {
		return renderCountText(d, table, where)
	}

	key := countShapeKey(d, table, where)

	if q, ok := shapeCacheGet(key); ok {
		return q, countArgs(where), nil
	}

	q, args, err := renderCountText(d, table, where)
	if err != nil {
		return "", nil, err
	}

	shapeCachePut(key, q)

	return q, args, nil
}

// Update renders an UPDATE statement with an optional ORDER BY / LIMIT /
// OFFSET tail and the same shape-cache treatment as Select; see Select's
// doc comment. Cacheable shapes (cacheableWhere AND cacheableOrder; an FTS
// order term renders through joinOrderBy and binds query arguments, so it
// always renders fresh) collect their bound args via updateArgs on a hit.
func Update(d dialect.Dialect, table string, sets []Assignment, where Node, order []OrderTerm, limit, offset int) (query string, args []any, err error) {
	if !cacheableWhere(where) || !cacheableOrder(order) {
		return renderUpdateText(d, table, sets, where, order, limit, offset)
	}

	key := updateShapeKey(d, table, sets, where, order, limit, offset)

	if q, ok := shapeCacheGet(key); ok {
		return q, updateArgs(sets, where, limit, offset), nil
	}

	q, args, err := renderUpdateText(d, table, sets, where, order, limit, offset)
	if err != nil {
		return "", nil, err
	}

	shapeCachePut(key, q)

	return q, args, nil
}

// Delete renders a DELETE statement with an optional ORDER BY / LIMIT /
// OFFSET tail and the same shape-cache treatment as Select; see Select's
// doc comment.
func Delete(d dialect.Dialect, table string, where Node, order []OrderTerm, limit, offset int) (query string, args []any, err error) {
	if !cacheableWhere(where) || !cacheableOrder(order) {
		return renderDeleteText(d, table, where, order, limit, offset)
	}

	key := deleteShapeKey(d, table, where, order, limit, offset)

	if q, ok := shapeCacheGet(key); ok {
		return q, deleteArgs(where, limit, offset), nil
	}

	q, args, err := renderDeleteText(d, table, where, order, limit, offset)
	if err != nil {
		return "", nil, err
	}

	shapeCachePut(key, q)

	return q, args, nil
}

// Insert renders a single-row INSERT (or DEFAULT VALUES) with the same
// shape-cache treatment as Select; see Select's doc comment.
func Insert(d dialect.Dialect, table string, columns []string, values []any) (query string, args []any, err error) {
	n := len(columns)
	if len(values) < n {
		n = len(values)
	}

	key := insertShapeKey(d, table, columns, n)

	if q, ok := shapeCacheGet(key); ok {
		return q, insertArgs(values, n), nil
	}

	q, args := renderInsertText(d, table, columns, values)

	shapeCachePut(key, q)

	return q, args, nil
}

// InsertMany renders a multi-row INSERT with the same shape-cache treatment
// as Select; see Select's doc comment. Malformed rows (a width mismatch
// against columns) never populate the cache: the key carries every row
// width, so a malformed call is a miss that still hits
// renderInsertManyText's validation error exactly as before.
func InsertMany(d dialect.Dialect, table string, columns []string, rows [][]any) (query string, args []any, err error) {
	key := insertManyShapeKey(d, table, columns, rows)

	if q, ok := shapeCacheGet(key); ok {
		return q, insertManyArgs(rows), nil
	}

	q, args, err := renderInsertManyText(d, table, columns, rows)
	if err != nil {
		return "", nil, err
	}

	shapeCachePut(key, q)

	return q, args, nil
}
