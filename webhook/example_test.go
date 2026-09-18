package webhook_test

import (
	"github.com/zenta-dev/zever/webhook"
	webhookhttp "github.com/zenta-dev/zever/webhook/http"
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
