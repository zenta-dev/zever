package webhook_test

import (
	webhookhttp "github.com/zenta-dev/zever/adapters/webhook/http"
	"github.com/zenta-dev/zever/core/webhook"
)

// ExampleOpen opens the http webhook backend with default options.
func ExampleOpen() {
	_ = webhook.Register(webhook.AdapterHTTP, webhookhttp.New)

	w, err := webhook.Open(webhook.AdapterHTTP, webhook.Options{})
	if err != nil {
		return
	}

	defer func() { _ = w.Close() }()
}
