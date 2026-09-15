package latex

import "errors"

// ErrMissingConverter is returned when image output needs a PDF converter but none resolves.
var ErrMissingConverter = errors.New("latex: pdf converter is required for image output")

// ErrRenderFailed is returned when the LaTeX toolchain fails to render the source.
var ErrRenderFailed = errors.New("latex: render failed")
