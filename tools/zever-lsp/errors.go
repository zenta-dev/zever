package main

import "errors"

// ErrRenameUnsupported is returned when rename is requested where it is not supported.
var ErrRenameUnsupported = errors.New("zever-lsp: rename not supported here")

// errRenameUnsupported aliases ErrRenameUnsupported for existing same-package callers.
var errRenameUnsupported = ErrRenameUnsupported

// ErrPublishDiagnostics reports a failure to deliver a publishDiagnostics
// notification for one file.
var ErrPublishDiagnostics = errors.New("zever-lsp: publish diagnostics failed")

// ErrExecuteUnknownCommand is returned when workspace/executeCommand names a
// command this server does not serve, or carries arguments the handler
// cannot use as a document reference.
var ErrExecuteUnknownCommand = errors.New("zever-lsp: unknown execute command")

// errExecuteUnknownCommand aliases ErrExecuteUnknownCommand for existing same-package callers.
var errExecuteUnknownCommand = ErrExecuteUnknownCommand
