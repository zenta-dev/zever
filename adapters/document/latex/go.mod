module github.com/zenta-dev/zever/adapters/document/latex

go 1.27.2

require github.com/zenta-dev/zever/core/document v0.8.0

require (
	github.com/zenta-dev/zever/shared/endpoint v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/ref v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/registry v0.8.0 // indirect
)

replace github.com/zenta-dev/zever/core/document => ../../../core/document

replace github.com/zenta-dev/zever/shared/endpoint => ../../../shared/endpoint

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/shared/ref => ../../../shared/ref
