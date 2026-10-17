module github.com/zenta-dev/zever/adapters/geo/static

go 1.27.2

require (
	github.com/zenta-dev/zever/core/geo v0.7.2
	github.com/zenta-dev/zever/shared/codec v0.7.2
)

require (
	github.com/zenta-dev/zever/shared/endpoint v0.7.2 // indirect
	github.com/zenta-dev/zever/shared/ref v0.7.2 // indirect
	github.com/zenta-dev/zever/shared/registry v0.7.2 // indirect
)

replace (
	github.com/zenta-dev/zever/core/geo => ../../../core/geo
	github.com/zenta-dev/zever/shared/codec => ../../../shared/codec
)

replace github.com/zenta-dev/zever/shared/endpoint => ../../../shared/endpoint

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/shared/ref => ../../../shared/ref
