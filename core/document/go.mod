module github.com/zenta-dev/zever/core/document

go 1.27.2

require (
	github.com/zenta-dev/zever/shared/endpoint v0.6.1
	github.com/zenta-dev/zever/shared/registry v0.6.1
)

replace (
	github.com/zenta-dev/zever/shared/endpoint => ../../shared/endpoint
	github.com/zenta-dev/zever/shared/registry => ../../shared/registry
)
