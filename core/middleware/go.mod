module github.com/zenta-dev/zever/core/middleware

go 1.27.0

require (
	github.com/zenta-dev/zever/adapters/log/noop v0.5.3
	github.com/zenta-dev/zever/core/log v0.5.3
	github.com/zenta-dev/zever/core/observability v0.5.3
	github.com/zenta-dev/zever/core/ratelimit v0.5.3
	github.com/zenta-dev/zever/shared/codec v0.5.3
	google.golang.org/grpc v1.84.0
)

require (
	github.com/zenta-dev/zever/shared/redisopt v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/registry v0.5.3 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260706201446-f0a921348800 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace (
	github.com/zenta-dev/zever/adapters/log/noop => ../../adapters/log/noop
	github.com/zenta-dev/zever/core/log => ../log
	github.com/zenta-dev/zever/core/observability => ../observability
	github.com/zenta-dev/zever/core/ratelimit => ../ratelimit
	github.com/zenta-dev/zever/shared/codec => ../../shared/codec
)

replace github.com/zenta-dev/zever/adapters/observability/noop => ../../adapters/observability/noop

replace github.com/zenta-dev/zever/adapters/ratelimit/memory => ../../adapters/ratelimit/memory

replace github.com/zenta-dev/zever/shared/redisopt => ../../shared/redisopt

replace github.com/zenta-dev/zever/shared/registry => ../../shared/registry
