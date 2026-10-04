package main

import "errors"

// ErrRenameUnsupported is returned when rename is requested where it is not supported.
var ErrRenameUnsupported = errors.New("zever-lsp: rename not supported here")

// errRenameUnsupported aliases ErrRenameUnsupported for existing same-package callers.
var errRenameUnsupported = ErrRenameUnsupported

// ErrPublishDiagnostics reports a failure to deliver a publishDiagnostics
// notification for one file.
var ErrPublishDiagnostics = errors.New("zever-lsp: publish diagnostics failed")
