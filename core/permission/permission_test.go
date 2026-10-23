package permission

import (
	"context"
	"sync/atomic"
)

var freshSeq atomic.Int64

type stubChecker struct{}

func (stubChecker) Can(_ context.Context, _ Subject, _ string, _ Resource) (Decision, error) {
	return Decision{Allowed: true, Reason: "allow"}, nil
}

