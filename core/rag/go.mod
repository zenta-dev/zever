module github.com/zenta-dev/zever/core/rag

go 1.27.2

require (
	github.com/zenta-dev/zever/core/agent v0.7.2
	github.com/zenta-dev/zever/core/ai v0.7.2
	github.com/zenta-dev/zever/core/search v0.7.2
	github.com/zenta-dev/zever/core/vectorstore v0.7.2
)

require (
	github.com/zenta-dev/zever/core/db v0.7.2 // indirect
	github.com/zenta-dev/zever/shared/endpoint v0.7.2 // indirect
	github.com/zenta-dev/zever/shared/mcpclient v0.7.2 // indirect
	github.com/zenta-dev/zever/shared/ref v0.7.2 // indirect
	github.com/zenta-dev/zever/shared/registry v0.7.2 // indirect
)

replace github.com/zenta-dev/zever/core/agent => ../agent

replace github.com/zenta-dev/zever/core/ai => ../ai

replace github.com/zenta-dev/zever/core/db => ../db

replace github.com/zenta-dev/zever/core/search => ../search

replace github.com/zenta-dev/zever/core/vectorstore => ../vectorstore

replace github.com/zenta-dev/zever/shared/endpoint => ../../shared/endpoint

replace github.com/zenta-dev/zever/shared/mcpclient => ../../shared/mcpclient

replace github.com/zenta-dev/zever/shared/registry => ../../shared/registry

replace github.com/zenta-dev/zever/shared/ref => ../../shared/ref
