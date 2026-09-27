module github.com/zenta-dev/zever/core/document

go 1.27.0

require (
	github.com/zenta-dev/zever/shared/endpoint v0.0.0
	github.com/zenta-dev/zever/shared/registry v0.0.0
)

replace (
	github.com/zenta-dev/zever/shared/endpoint => ../../shared/endpoint
	github.com/zenta-dev/zever/shared/registry => ../../shared/registry
)
