package cdc

import "errors"

// ErrClosed is returned by Start after Close has stopped the store.
var ErrClosed = errors.New("cdc: outbox closed")
