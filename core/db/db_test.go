package db

type stubRows struct{}

func (r *stubRows) Next() bool                 { return false }
func (r *stubRows) Scan(_ ...any) error        { return nil }
func (r *stubRows) Close() error               { return nil }
func (r *stubRows) Columns() ([]string, error) { return []string{"id"}, nil }
func (r *stubRows) Err() error                 { return nil }
