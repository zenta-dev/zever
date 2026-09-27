module github.com/zenta-dev/zever/adapters/geo/google

go 1.27.0

require (
	github.com/zenta-dev/zever/core/geo v0.0.0
	github.com/zenta-dev/zever/shared/endpoint v0.0.0
	github.com/zenta-dev/zever/shared/httpclient v0.0.0
	googlemaps.github.io/maps v1.7.0
)

require (
	github.com/google/uuid v1.6.0 // indirect
	github.com/zenta-dev/zever/shared/registry v0.0.0 // indirect
	go.opencensus.io v0.24.0 // indirect
	golang.org/x/time v0.15.0 // indirect
)

replace github.com/zenta-dev/zever/core/geo => ../../../core/geo

replace github.com/zenta-dev/zever/shared/endpoint => ../../../shared/endpoint

replace github.com/zenta-dev/zever/shared/httpclient => ../../../shared/httpclient

replace github.com/zenta-dev/zever/adapters/geo/static => ../static

replace github.com/zenta-dev/zever/shared/codec => ../../../shared/codec

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
