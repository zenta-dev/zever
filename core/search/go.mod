module github.com/zenta-dev/zever/core/search

go 1.27.0

require (
	github.com/zenta-dev/zever/core/db v0.5.3
	github.com/zenta-dev/zever/shared/registry v0.5.3
)

replace github.com/zenta-dev/zever/core/db => ../db

replace github.com/zenta-dev/zever/shared/registry => ../../shared/registry
