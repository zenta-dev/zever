package latex

import "errors"

// ErrMissingConverter is returned when image output needs a PDF converter but none resolves.
var ErrMissingConverter = errors.New("latex: pdf converter is required for image output")

// ErrRenderFailed is returned when the LaTeX toolchain fails to render the source.
var ErrRenderFailed = errors.New("latex: render failed")

// ErrUnsafeSource is returned when the source pre-scan finds a file access
// outside the render directory or a shell-escape package. Disable the scan
// with the latex_allow_unsafe_source option for fully trusted sources.
var ErrUnsafeSource = errors.New("latex: unsafe source")

// ErrSandboxUnavailable is returned by New when latex_sandbox is "required"
// but no working OS sandbox (bubblewrap) is available.
var ErrSandboxUnavailable = errors.New("latex: OS sandbox required but unavailable")
