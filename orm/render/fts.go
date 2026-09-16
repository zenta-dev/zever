// Package render (this file) extends the package with full-text-search
// expression rendering. An NFTS node's base column (Table/Column) is
// rendered as a per-dialect full-text expression -- Postgres's tsvector
// machinery (`to_tsvector('english', col) @@ plainto_tsquery('english',
// ?)`) or SQLite's FTS5 MATCH operator (`col MATCH ?`) -- and an FTS
// ranking OrderTerm renders `ts_rank(...)` on Postgres. See render.go's
// KindFTS.
package render

import (
	"github.com/zenta-dev/zever/orm/dialect"
)

// renderFTS renders one NFTS predicate node: the full-text expression over
// the base column. The rendering is per-dialect: a node carries an abstract
// operator, not a dialect's syntax. The postgres/sqlite syntax families
// genuinely differ (tsvector machinery vs FTS5 MATCH), so this is one of the
// few places render branches on dialect identity; any other dialect renders
// the postgres form, which a database lacking those functions rejects loudly
// rather than silently returning wrong rows.
func renderFTS(d dialect.Dialect, n Node, counter *argCounter) (clause string, args []any, err error) {
	if n.FTS == nil {
		return "", nil, nil
	}

	if isSQLite(d) {
		ph := d.Placeholder(counter.next())

		return quoteColumn(d, n.Column) + " MATCH " + ph, []any{n.FTS.Query}, nil
	}

	col := quoteColumn(d, n.Column)

	queryFn := "plainto_tsquery"
	if n.FTS.Op == FTSMatchTSQuery {
		queryFn = "to_tsquery"
	}

	ph := d.Placeholder(counter.next())

	return "to_tsvector('english', " + col + ") @@ " + queryFn + "('english', " + ph + ")", []any{n.FTS.Query}, nil
}

// ftsOrderExpr renders an FTS ranking OrderTerm's expression, continuing
// counter for its bound placeholder:
//
//	Postgres: ts_rank(to_tsvector('english', <col>), plainto_tsquery('english', ?))
//
// The Postgres form lets callers order by FULLTEXT relevance. Only FTSRank
// terms reach here; anything else renders the plain quoted column.
func ftsOrderExpr(d dialect.Dialect, o OrderTerm, counter *argCounter) (string, []any) {
	if o.FTS != nil && o.FTS.Op == FTSRank {
		col := quoteColumn(d, o.Column)
		ph := d.Placeholder(counter.next())

		return "ts_rank(to_tsvector('english', " + col + "), plainto_tsquery('english', " + ph + "))", []any{o.FTS.Query}
	}

	return quoteColumn(d, o.Column), nil
}
