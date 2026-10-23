package eventbus

import (
	"context"
	"sync/atomic"
)

var freshSeq atomic.Int64

type stubBus struct{}

func (stubBus) Publish(_ context.Context, _ string, _ Payload, _ Headers) error {
	return nil
}

func (stubBus) Subscribe(_ context.Context, _ string, _ Handler) (func(), error) {
	return func() {}, nil
}

func (stubBus) Close() error { return nil }

func (stubBus) Name() string { return "stub" }

func (stubBus) SubscribeChan(_ context.Context, _ string, _ int) (<-chan Message, error) {
	return nil, ErrNotSubscribed
}

func (stubBus) Unsubscribe(_ string, _ <-chan Message) error { return ErrNotSubscribed }

