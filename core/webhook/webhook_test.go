package webhook

import (
	"context"
	"testing"
)

type stubWebhook struct {
	event   string
	target  string
	secret  string
	payload []byte
}

func (s *stubWebhook) Register(_ context.Context, event, target, secret string) error {
	s.event = event
	s.target = target
	s.secret = secret
	return nil
}

func (s *stubWebhook) Unregister(_ context.Context, event, target string) error {
	s.event = event
	s.target = target
	return nil
}

func (s *stubWebhook) Deliver(_ context.Context, event string, payload []byte) error {
	s.event = event
	s.payload = payload
	return nil
}

func (s *stubWebhook) Close() error { return nil }

func TestWebhook_stub_smoke(t *testing.T) {
	t.Parallel()

	stub := &stubWebhook{}
	var w Webhook = stub
	ctx := t.Context()
	if err := w.Register(ctx, "order.created", "https://example.com/hook", "s3cr3t"); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	if stub.event != "order.created" || stub.target != "https://example.com/hook" || stub.secret != "s3cr3t" {
		t.Fatalf("stub recorded %+v, want event/target/secret", stub)
	}
	if err := w.Deliver(ctx, "order.created", []byte(`{}`)); err != nil {
		t.Fatalf("Deliver err = %v", err)
	}
	if string(stub.payload) != `{}` {
		t.Fatalf("stub payload = %q want %q", stub.payload, `{}`)
	}
	if err := w.Unregister(ctx, "order.created", "https://example.com/hook"); err != nil {
		t.Fatalf("Unregister err = %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}
}
