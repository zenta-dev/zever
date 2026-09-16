package postgres

// SupportsWindowFrameRows reports that Postgres supports a `ROWS` window
// frame. Postgres implements all three frame modes (ROWS/RANGE/GROUPS) at
// every supported version.
func (Dialect) SupportsWindowFrameRows() bool { return true }

// SupportsWindowFrameRange reports that Postgres supports a `RANGE` window
// frame.
func (Dialect) SupportsWindowFrameRange() bool { return true }

// SupportsWindowFrameGroups reports that Postgres supports a `GROUPS`
// window frame.
func (Dialect) SupportsWindowFrameGroups() bool { return true }
