module github.com/zenta-dev/zever/adapters/workflow/memory

go 1.27.3

require (
	github.com/zenta-dev/zever/core/workflow v0.8.0
	github.com/zenta-dev/zever/shared/codec v0.8.0
	github.com/zenta-dev/zever/shared/retry v0.8.0
)

require (
	github.com/zenta-dev/zever/core/db v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/ref v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/registry v0.8.0 // indirect
)

replace github.com/zenta-dev/zever/core/workflow => ../../../core/workflow

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/shared/retry => ../../../shared/retry

replace github.com/zenta-dev/zever/core/db => ../../../core/db

replace github.com/zenta-dev/zever/shared/ref => ../../../shared/ref

replace github.com/zenta-dev/zever/shared/codec => ../../../shared/codec
