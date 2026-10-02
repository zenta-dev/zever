package dbconn

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	coredb "github.com/zenta-dev/zever/core/db"
)

// ErrInvalidTableName is returned by ValidateTableName when name falls
// outside [A-Za-z_][A-Za-z0-9_]*. It carries no battery prefix so every
// DB-backed adapter shares one sentinel; callers wrap it with their own
// prefix (for example fmt.Errorf("db: %w", err)) to keep error text
// grep-able per battery.
var ErrInvalidTableName = errors.New("dbconn: invalid table name")

// IsPostgresDSN reports whether dsn selects the postgres backend: a URL
// with a postgres scheme. Anything else (including ":memory:") is a sqlite
// path. Matching is case-insensitive with surrounding whitespace ignored.
func IsPostgresDSN(dsn string) bool {
	s := strings.ToLower(strings.TrimSpace(dsn))

	return strings.HasPrefix(s, "postgres://") || strings.HasPrefix(s, "postgresql://")
}

// SplitDSN maps a single-string core DSN onto shared pool options: a
// postgres URL stays a DSN, anything else (including empty) becomes a
// sqlite Path. Empty selects ":memory:".
func SplitDSN(dsn string) coredb.Options {
	if IsPostgresDSN(dsn) {
		return coredb.Options{DSN: dsn}
	}

	if strings.TrimSpace(dsn) == "" {
		return coredb.Options{Path: ":memory:"}
	}

	return coredb.Options{Path: dsn}
}

// ValidateTableName rejects table names outside [A-Za-z_][A-Za-z0-9_]*
// because the name is interpolated into DDL (orm has no DDL builder). The
// returned error wraps ErrInvalidTableName; callers add their battery
// prefix with %w.
func ValidateTableName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: table must not be empty", ErrInvalidTableName)
	}

	for i := 0; i < len(name); i++ {
		c := name[i]
		ok := c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (i > 0 && c >= '0' && c <= '9')

		if !ok {
			return fmt.Errorf("%w: %q", ErrInvalidTableName, name)
		}
	}

	return nil
}

// RandomOwner mints a unique claim-holder name for one driver instance:
// prefix plus 16 hex digits from crypto/rand. Owners must differ per
// replica that may reclaim work from a shared database; a static default
// would let two replicas share one identity and disturb each other's
// leases. crypto/rand failure is impossible to surface usefully here, so
// a short fallback keeps construction infallible.
func RandomOwner(prefix string) string {
	var b [8]byte

	if _, err := rand.Read(b[:]); err != nil {
		return prefix + "-fallback"
	}

	return prefix + "-" + hex.EncodeToString(b[:])
}
