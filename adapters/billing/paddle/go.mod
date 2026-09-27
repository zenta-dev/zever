module github.com/zenta-dev/zever/adapters/billing/paddle

go 1.27.0

require (
	github.com/PaddleHQ/paddle-go-sdk/v5 v5.2.0
	github.com/zenta-dev/zever/core/billing v0.0.0
	github.com/zenta-dev/zever/shared/httpclient v0.0.0
	github.com/zenta-dev/zever/shared/providersopt v0.0.0
)

require (
	github.com/ggicci/httpin v0.20.3 // indirect
	github.com/ggicci/owl v0.8.2 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/zenta-dev/zever/shared/registry v0.0.0 // indirect
)

replace github.com/zenta-dev/zever/shared/httpclient => ../../../shared/httpclient

replace github.com/zenta-dev/zever/shared/providersopt => ../../../shared/providersopt

replace github.com/zenta-dev/zever/core/billing => ../../../core/billing

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
