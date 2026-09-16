package sqlite

// SupportsWindowFrameRows reports whether this SQLite library version
// supports a `ROWS` window frame. Window frames arrived in SQLite 3.28.0,
// so this reports true only for >= 3.28.0. New defaults to 3.46.0, so the
// common path reports true.
func (d Dialect) SupportsWindowFrameRows() bool {
	return d.version.atLeast(3, 28, 0)
}

// SupportsWindowFrameRange reports whether this SQLite library version
// supports a `RANGE` window frame. See SupportsWindowFrameRows (3.28.0).
func (d Dialect) SupportsWindowFrameRange() bool {
	return d.version.atLeast(3, 28, 0)
}

// SupportsWindowFrameGroups reports whether this SQLite library version
// supports a `GROUPS` window frame. See SupportsWindowFrameRows (3.28.0).
func (d Dialect) SupportsWindowFrameGroups() bool {
	return d.version.atLeast(3, 28, 0)
}
