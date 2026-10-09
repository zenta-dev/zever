module github.com/zenta-dev/zever/adapters/geo/static

go 1.27.2

require (
	github.com/zenta-dev/zever/core/geo v0.6.1
	github.com/zenta-dev/zever/shared/codec v0.6.1
)

require (
	github.com/zenta-dev/zever/shared/endpoint v0.6.1 // indirect
	github.com/zenta-dev/zever/shared/registry v0.6.1 // indirect
)

replace (
	github.com/zenta-dev/zever/core/geo => ../../../core/geo
	github.com/zenta-dev/zever/shared/codec => ../../../shared/codec
)

replace github.com/zenta-dev/zever/shared/endpoint => ../../../shared/endpoint

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
