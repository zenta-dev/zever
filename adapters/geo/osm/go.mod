module github.com/zenta-dev/zever/adapters/geo/osm

go 1.27.0

require (
	github.com/zenta-dev/zever/core/geo v0.0.0
	github.com/zenta-dev/zever/shared/codec v0.0.0
	github.com/zenta-dev/zever/shared/endpoint v0.0.0
	github.com/zenta-dev/zever/shared/httpclient v0.0.0
)

require github.com/zenta-dev/zever/shared/registry v0.0.0 // indirect

replace (
	github.com/zenta-dev/zever/core/geo => ../../../core/geo
	github.com/zenta-dev/zever/shared/codec => ../../../shared/codec
	github.com/zenta-dev/zever/shared/endpoint => ../../../shared/endpoint
	github.com/zenta-dev/zever/shared/httpclient => ../../../shared/httpclient
)

replace github.com/zenta-dev/zever/adapters/geo/static => ../static

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
