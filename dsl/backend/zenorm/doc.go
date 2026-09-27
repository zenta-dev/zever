// Package zenorm implements the "zenorm" backend.Backend: it renders a
// resolved *ir.Schema into one Go source file per ir.Module, emitting the
// orm (github.com/zenta-dev/zever/orm) package's Table/Column/
// NullableColumn vars, the plain entity struct, and a positional
// zero-reflection Scan method.
package zenorm
