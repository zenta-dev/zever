package queue

import (
	"context"
	"time"
)

type stubQueue struct{}

var _ Queue = stubQueue{}

func (stubQueue) Push(_ context.Context, _ string, _ Payload, _ Headers) error { return nil }

func (stubQueue) PushDelayed(_ context.Context, _ string, _ Payload, _ Headers, _ time.Duration) error {
	return nil
}

func (stubQueue) Pop(_ context.Context, _ string) (Message, error) { return Message{}, nil }

func (stubQueue) Ack(_ context.Context, _ Message) error { return nil }

func (stubQueue) Nack(_ context.Context, _ Message, _ bool) error { return nil }

func (stubQueue) Length(_ context.Context, _ string) (int64, error) { return 0, nil }

func (stubQueue) IsEmpty(_ context.Context, _ string) (bool, error) { return true, nil }

func (stubQueue) Close() error { return nil }

func (stubQueue) Name() string { return "" }
