module github.com/zenta-dev/zever/core/authz

go 1.27.2

require (
	github.com/zenta-dev/zever/core/auth v0.6.1
	github.com/zenta-dev/zever/core/permission v0.6.1
	github.com/zenta-dev/zever/shared/codec v0.6.1
	google.golang.org/grpc v1.83.2
	google.golang.org/protobuf v1.36.12
)

require (
	github.com/zenta-dev/zever/core/db v0.6.1 // indirect
	github.com/zenta-dev/zever/core/session v0.6.1 // indirect
	github.com/zenta-dev/zever/shared/endpoint v0.6.1 // indirect
	github.com/zenta-dev/zever/shared/redisopt v0.6.1 // indirect
	github.com/zenta-dev/zever/shared/registry v0.6.1 // indirect
	golang.org/x/net v0.60.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260526163538-3dc84a4a5aaa // indirect
)

replace (
	github.com/zenta-dev/zever/core/auth => ../auth
	github.com/zenta-dev/zever/core/permission => ../permission
	github.com/zenta-dev/zever/shared/codec => ../../shared/codec
)

replace github.com/zenta-dev/zever/adapters/auth/session => ../../adapters/auth/session

replace github.com/zenta-dev/zever/adapters/permission/noop => ../../adapters/permission/noop

replace github.com/zenta-dev/zever/adapters/session/memory => ../../adapters/session/memory

replace github.com/zenta-dev/zever/core/session => ../session

replace github.com/zenta-dev/zever/shared/redisclient => ../../shared/redisclient

replace github.com/zenta-dev/zever/shared/redisopt => ../../shared/redisopt

replace github.com/zenta-dev/zever/shared/registry => ../../shared/registry

replace github.com/zenta-dev/zever/core/db => ../db

replace github.com/zenta-dev/zever/shared/endpoint => ../../shared/endpoint
