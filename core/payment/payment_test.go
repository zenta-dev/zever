package payment

import (
	"context"
	"sync/atomic"
)

var testSeq atomic.Int64

type stubPayment struct {
	result Result
}

func (s *stubPayment) CreatePayment(_ context.Context, _ Request) (Result, error) {
	return s.result, nil
}

func (s *stubPayment) Refund(_ context.Context, _ string, _ int64, _ string) error {
	return nil
}

func (s *stubPayment) GetPayment(_ context.Context, _ string) (Result, error) {
	return s.result, nil
}

func (s *stubPayment) WebhookEvent(_ context.Context, _ []byte, _ string) (Event, error) {
	return Event{Type: EventType("stub.event"), Object: s.result}, nil
}

func (s *stubPayment) Close() error {
	return nil
}

