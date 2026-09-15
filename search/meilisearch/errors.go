package meilisearch

import "errors"

// ErrMissingHost is returned when opening a Meilisearch backend without a host.
var ErrMissingHost = errors.New("meilisearch: host is required")

// ErrIndexRequired is returned when searching without an explicit index filter.
var ErrIndexRequired = errors.New("meilisearch: an explicit index filter is required; refusing to search across all indexes")
