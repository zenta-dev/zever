package db

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/outbox"
)

// errPublish is the injected publisher failure.
var errPublish = errors.New("db: test publish failed")

// errNil is a package-level error variable returned by test callbacks so
// unparam does not treat those callbacks as always-nil.
var errNil error

// recordingPublisher records delivered messages and can fail the next n
// publish calls.
type recordingPublisher struct {
	mu       sync.Mutex
	msgs     []outbox.Message
	failures int
}

func (p *recordingPublisher) Publish(_ context.Context, msg outbox.Message) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.failures > 0 {
		p.failures--

		return errPublish
	}

	p.msgs = append(p.msgs, msg.Clone())

	return nil
}

func (p *recordingPublisher) messages() []outbox.Message {
	p.mu.Lock()
	defer p.mu.Unlock()

	out := make([]outbox.Message, len(p.msgs))
	copy(out, p.msgs)

	return out
}

func (p *recordingPublisher) failNext(n int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.failures = n
}

// openSQLite returns a raw sqlite DB backed by a fresh temp file.
func openSQLite(t *testing.T) (coredb.DB, error) {
	t.Helper()

	return dbsqlite.New(coredb.Options{Path: filepath.Join(t.TempDir(), "shared.db")})
}
