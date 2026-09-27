module github.com/zenta-dev/zever/shared/providersclient

go 1.27.0

require (
	github.com/stripe/stripe-go/v82 v82.5.1
	github.com/zenta-dev/zever/shared/httpclient v0.0.0
)

replace github.com/zenta-dev/zever/shared/httpclient => ../httpclient
