module github.com/zenta-dev/zever/adapters/geo/static

go 1.27.3

require (
	github.com/zenta-dev/zever/core/geo v0.8.0
	github.com/zenta-dev/zever/shared/codec v0.8.0
)

require (
	github.com/zenta-dev/zever/shared/endpoint v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/ref v0.8.0 // indirect
)

replace (
	github.com/zenta-dev/zever/core/geo => ../../../core/geo
	github.com/zenta-dev/zever/shared/codec => ../../../shared/codec
)

replace github.com/zenta-dev/zever/shared/endpoint => ../../../shared/endpoint

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/shared/ref => ../../../shared/ref
