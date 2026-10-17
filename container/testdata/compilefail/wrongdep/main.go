// Fixture wrongdep must NOT compile: the dependency Ref's value type
// (cache.Cache) does not match the factory's dependency parameter
// (queue.Queue), so Provide1 has no valid type instantiation.
//
// Enforced locally with `go build ./...` in this directory (and by the
// container compile-fail test). The full CI job wiring these fixtures
// lands in stream G; until then this directory plus the test are the gate.
//
// NOTE: this module is intentionally excluded from the workspace (no `use`
// line in the root go.work) and lives under testdata, so the go tool
// ignores it during normal builds.
package main

import (
	"github.com/zenta-dev/zever/container"
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/shared/ref"
)

var _ = container.Provide1(
	coredb.Ref("analytics"),
	ref.Named[queue.Queue, queue.Options]("queue", "default"),
	func(o coredb.Options, q *struct{ Nope int }) (coredb.DB, error) {
		return nil, nil
	},
)

func main() {}
