module github.com/zenta-dev/zever/core/workflow

go 1.27.2

require (
	github.com/zenta-dev/zever/adapters/workflow/memory v0.7.2
	github.com/zenta-dev/zever/core/db v0.7.2
	github.com/zenta-dev/zever/shared/registry v0.7.2
)

require github.com/zenta-dev/zever/shared/retry v0.7.2 // indirect

replace github.com/zenta-dev/zever/core/db => ../db

replace github.com/zenta-dev/zever/shared/registry => ../../shared/registry

replace github.com/zenta-dev/zever/adapters/workflow/memory => ../../adapters/workflow/memory

replace github.com/zenta-dev/zever/shared/retry => ../../shared/retry
