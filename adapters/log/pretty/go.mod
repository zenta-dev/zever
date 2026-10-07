module github.com/zenta-dev/zever/adapters/log/pretty

go 1.27.0

require (
	github.com/zenta-dev/zever/core/log v0.6.0
	github.com/zenta-dev/zever/core/observability v0.6.0
)

require github.com/zenta-dev/zever/shared/registry v0.5.3 // indirect

replace (
	github.com/zenta-dev/zever/core/log => ../../../core/log
	github.com/zenta-dev/zever/core/observability => ../../../core/observability
)

replace github.com/zenta-dev/zever/adapters/log/noop => ../noop

replace github.com/zenta-dev/zever/adapters/observability/noop => ../../observability/noop

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
