module github.com/zenta-dev/zever/adapters/document/remote

go 1.27.0

require (
	github.com/zenta-dev/zever/core/document v0.0.0
	github.com/zenta-dev/zever/shared/codec v0.0.0
	github.com/zenta-dev/zever/shared/endpoint v0.0.0
	github.com/zenta-dev/zever/shared/httpclient v0.0.0
)

require github.com/zenta-dev/zever/shared/registry v0.0.0 // indirect

replace (
	github.com/zenta-dev/zever/core/document => ../../../core/document
	github.com/zenta-dev/zever/shared/codec => ../../../shared/codec
	github.com/zenta-dev/zever/shared/endpoint => ../../../shared/endpoint
	github.com/zenta-dev/zever/shared/httpclient => ../../../shared/httpclient
)

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
