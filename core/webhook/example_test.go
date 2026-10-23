package webhook_test

import (
	webhookhttp "github.com/zenta-dev/zever/adapters/webhook/http"
	"github.com/zenta-dev/zever/core/webhook"
)

// Example opens the http webhook backend with default options.
func Example() {
	w, err := webhookhttp.New(webhook.Options{})
	if err != nil {
		return
	}

	defer func() { _ = w.Close() }()
}
