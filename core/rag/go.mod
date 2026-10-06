module github.com/zenta-dev/zever/core/rag

go 1.27.0

require (
	github.com/zenta-dev/zever/core/ai v0.5.3
	github.com/zenta-dev/zever/core/search v0.5.3
	github.com/zenta-dev/zever/core/vectorstore v0.5.3
)

require (
	github.com/zenta-dev/zever/core/db v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/endpoint v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/registry v0.5.3 // indirect
)

replace github.com/zenta-dev/zever/core/ai => ../ai

replace github.com/zenta-dev/zever/core/db => ../db

replace github.com/zenta-dev/zever/core/search => ../search

replace github.com/zenta-dev/zever/core/vectorstore => ../vectorstore

replace github.com/zenta-dev/zever/shared/endpoint => ../../shared/endpoint

replace github.com/zenta-dev/zever/shared/registry => ../../shared/registry
