module github.com/zenta-dev/zever/adapters/observability/stdout

go 1.27.2

require (
	github.com/zenta-dev/zever/core/observability v0.6.1
	github.com/zenta-dev/zever/shared/codec v0.6.1
)

require github.com/zenta-dev/zever/shared/registry v0.6.1 // indirect

replace (
	github.com/zenta-dev/zever/core/observability => ../../../core/observability
	github.com/zenta-dev/zever/shared/codec => ../../../shared/codec
)

replace github.com/zenta-dev/zever/adapters/observability/noop => ../noop

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
