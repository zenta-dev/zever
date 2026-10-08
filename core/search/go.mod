module github.com/zenta-dev/zever/core/search

go 1.27.0

require (
	github.com/zenta-dev/zever/core/db v0.6.0
	github.com/zenta-dev/zever/shared/endpoint v0.6.0
	github.com/zenta-dev/zever/shared/registry v0.6.0
)

replace github.com/zenta-dev/zever/core/db => ../db

replace github.com/zenta-dev/zever/shared/endpoint => ../../shared/endpoint

replace github.com/zenta-dev/zever/shared/registry => ../../shared/registry
