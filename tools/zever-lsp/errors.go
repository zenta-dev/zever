package main

import "errors"

// errRenameUnsupported is returned when rename is requested where it is not supported.
var errRenameUnsupported = errors.New("zever-lsp: rename not supported here")
