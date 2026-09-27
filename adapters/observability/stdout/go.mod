module github.com/zenta-dev/zever/adapters/observability/stdout

go 1.27.0

require (
	github.com/zenta-dev/zever/core/observability v0.0.0
	github.com/zenta-dev/zever/shared/codec v0.0.0
)

require github.com/zenta-dev/zever/shared/registry v0.0.0 // indirect

replace (
	github.com/zenta-dev/zever/core/observability => ../../../core/observability
	github.com/zenta-dev/zever/shared/codec => ../../../shared/codec
)

replace github.com/zenta-dev/zever/adapters/observability/noop => ../noop

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
