module github.com/zenta-dev/zever/core/document

go 1.27.0

require (
	github.com/zenta-dev/zever/shared/endpoint v0.5.3
	github.com/zenta-dev/zever/shared/registry v0.5.3
)

replace (
	github.com/zenta-dev/zever/shared/endpoint => ../../shared/endpoint
	github.com/zenta-dev/zever/shared/registry => ../../shared/registry
)
