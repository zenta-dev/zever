package cas

// Package cas shares compare-and-swap Lua scripts for cache and lock Redis
// adapters. One copy keeps delete-if-owner and expire-if-owner atomic and
// identical across batteries.
