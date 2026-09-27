package sms_test

import (
	"encoding/json"
	"strings"
	"testing"

	sms "github.com/example/zever-sms"
	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
)

func TestEndToEnd(t *testing.T) {
	t.Parallel()
	if err := sms.RegisterPlugin(); err != nil {
		t.Fatalf("RegisterPlugin: %v", err)
	}

	cfg := config.Default()
	cfg.Plugins = map[string]config.Service[json.RawMessage]{
		"sms": {
			Adapter: "stub",
			Options: json.RawMessage(`{"from":"+15550000"}`),
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	c := container.New(cfg)
	svc, err := container.Resolve[sms.SMS](c, "sms")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if err := svc.Send(t.Context(), "+15550001", "hello from external sms"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	senter, ok := svc.(interface{ Sent() []sms.Message })
	if !ok {
		t.Fatal("resolved SMS does not expose Sent() for inspection")
	}
	sent := senter.Sent()
	if len(sent) != 1 {
		t.Fatalf("sent = %d messages, want 1", len(sent))
	}
	if sent[0].To != "+15550001" || sent[0].Body != "hello from external sms" {
		t.Fatalf("sent[0] = %+v, want to=+15550001 body=hello from external sms", sent[0])
	}

	again, err := container.Resolve[sms.SMS](c, "sms")
	if err != nil {
		t.Fatalf("second Resolve: %v", err)
	}
	if err := again.Send(t.Context(), "+15550002", "second"); err != nil {
		t.Fatalf("second Send: %v", err)
	}
	if got := senter.Sent(); len(got) != 2 {
		t.Fatalf("sent after second = %d, want 2 (per-container cached instance)", len(got))
	}
}

func TestVersionMismatch(t *testing.T) {
	t.Parallel()
	err := container.RegisterPlugin[sms.SMS](
		"sms-external-version-mismatch",
		container.PluginAPIVersion+1,
		func(*config.Config) (sms.SMS, error) { return nil, nil },
	)
	if err == nil {
		t.Fatal("RegisterPlugin bad version: got nil, want error")
	}
	if !strings.Contains(err.Error(), "version mismatch") {
		t.Fatalf("version error = %q, want version mismatch", err)
	}
}
