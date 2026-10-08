module github.com/zenta-dev/zever/adapters/workflow/memory

go 1.27.0

require (
	github.com/zenta-dev/zever/core/workflow v0.6.1
	github.com/zenta-dev/zever/shared/retry v0.6.1
)

require (
	github.com/zenta-dev/zever/core/db v0.6.1 // indirect
	github.com/zenta-dev/zever/shared/registry v0.6.1 // indirect
)

replace github.com/zenta-dev/zever/core/workflow => ../../../core/workflow

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/shared/retry => ../../../shared/retry

replace github.com/zenta-dev/zever/core/db => ../../../core/db
