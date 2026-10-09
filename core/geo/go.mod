module github.com/zenta-dev/zever/core/geo

go 1.27.2

require (
	github.com/zenta-dev/zever/adapters/geo/static v0.6.1
	github.com/zenta-dev/zever/shared/endpoint v0.6.1
	github.com/zenta-dev/zever/shared/registry v0.6.1
)

require github.com/zenta-dev/zever/shared/codec v0.6.1 // indirect

replace (
	github.com/zenta-dev/zever/adapters/geo/static => ../../adapters/geo/static
	github.com/zenta-dev/zever/shared/endpoint => ../../shared/endpoint
	github.com/zenta-dev/zever/shared/registry => ../../shared/registry
)

replace github.com/zenta-dev/zever/shared/codec => ../../shared/codec
