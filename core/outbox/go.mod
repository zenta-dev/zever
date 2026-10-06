module github.com/zenta-dev/zever/core/outbox

go 1.27.0

require (
	github.com/zenta-dev/zever/core/db v0.5.3
	github.com/zenta-dev/zever/core/observability v0.5.3
	github.com/zenta-dev/zever/shared/registry v0.5.3
	github.com/zenta-dev/zever/shared/retry v0.5.3
)

replace github.com/zenta-dev/zever/core/db => ../db

replace github.com/zenta-dev/zever/core/observability => ../observability

replace github.com/zenta-dev/zever/shared/registry => ../../shared/registry

replace github.com/zenta-dev/zever/shared/retry => ../../shared/retry
