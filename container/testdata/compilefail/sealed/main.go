// Fixture sealed must NOT compile: container.Provider is sealed by its
// unexported method, so an external implementation cannot satisfy the
// interface.
//
// Enforced locally with `go build ./...` in this directory (and by the
// container compile-fail test). The full CI job wiring these fixtures
// lands in stream G; until then this directory plus the test are the gate.
//
// NOTE: this module is intentionally excluded from the workspace (no `use`
// line in the root go.work) and lives under testdata, so the go tool
// ignores it during normal builds.
package main

import "github.com/zenta-dev/zever/container"

// handProvider tries to implement container.Provider by hand.
type handProvider struct{}

// sealProvider references the unexported sealer, which never compiles
// outside the container package.
func (handProvider) sealProvider() {}

var _ container.Provider = handProvider{}

func main() {}
