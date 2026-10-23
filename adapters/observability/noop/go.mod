module github.com/zenta-dev/zever/adapters/observability/noop

go 1.27.3

require github.com/zenta-dev/zever/core/observability v0.8.0

require github.com/zenta-dev/zever/shared/ref v0.8.0 // indirect

replace github.com/zenta-dev/zever/core/observability => ../../../core/observability

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/shared/ref => ../../../shared/ref
