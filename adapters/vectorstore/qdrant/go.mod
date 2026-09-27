module github.com/zenta-dev/zever/adapters/vectorstore/qdrant

go 1.27.0

require (
	github.com/zenta-dev/zever/shared/registry v0.0.0 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260819154853-08b0e4226688 // indirect
	google.golang.org/grpc v1.83.2 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
)

replace github.com/zenta-dev/zever/adapters/vectorstore/sqlite => ../sqlite

require (
	github.com/qdrant/go-client v1.19.2
	github.com/zenta-dev/zever/core/vectorstore v0.0.0
)

replace github.com/zenta-dev/zever/core/vectorstore => ../../../core/vectorstore

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
