module github.com/zenta-dev/zever/adapters/billing/stripe

go 1.27.0

require (
	github.com/stripe/stripe-go/v82 v82.5.1
	github.com/zenta-dev/zever/core/billing v0.0.0
	github.com/zenta-dev/zever/shared/providersclient v0.0.0
)

require (
	github.com/zenta-dev/zever/shared/httpclient v0.0.0 // indirect
	github.com/zenta-dev/zever/shared/providersopt v0.0.0 // indirect
	github.com/zenta-dev/zever/shared/registry v0.0.0 // indirect
)

replace github.com/zenta-dev/zever/shared/providersclient => ../../../shared/providersclient

replace github.com/zenta-dev/zever/shared/httpclient => ../../../shared/httpclient

replace github.com/zenta-dev/zever/shared/providersopt => ../../../shared/providersopt

replace github.com/zenta-dev/zever/core/billing => ../../../core/billing

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
