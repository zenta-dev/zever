module github.com/zenta-dev/zever/adapters/outbox/memory

go 1.27.0

require (
	github.com/zenta-dev/zever/core/db v0.5.3
	github.com/zenta-dev/zever/core/outbox v0.5.3
)

require (
	github.com/zenta-dev/zever/core/observability v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/registry v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/retry v0.5.3 // indirect
)

replace github.com/zenta-dev/zever/core/db => ../../../core/db

replace github.com/zenta-dev/zever/core/outbox => ../../../core/outbox

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/shared/retry => ../../../shared/retry
