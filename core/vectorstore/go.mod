module github.com/zenta-dev/zever/core/vectorstore

go 1.27.2

require (
	github.com/zenta-dev/zever/core/db v0.7.2
	github.com/zenta-dev/zever/shared/endpoint v0.7.2
	github.com/zenta-dev/zever/shared/ref v0.7.2
	github.com/zenta-dev/zever/shared/registry v0.7.2
)

replace github.com/zenta-dev/zever/core/db => ../db

replace github.com/zenta-dev/zever/shared/endpoint => ../../shared/endpoint

replace github.com/zenta-dev/zever/shared/registry => ../../shared/registry

replace github.com/zenta-dev/zever/shared/ref => ../../shared/ref
