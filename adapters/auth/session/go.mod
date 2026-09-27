module github.com/zenta-dev/zever/adapters/auth/session

go 1.27.0

require (
	github.com/zenta-dev/zever/adapters/session/memory v0.0.0
	github.com/zenta-dev/zever/core/auth v0.0.0
	github.com/zenta-dev/zever/core/session v0.0.0
)

require (
	github.com/zenta-dev/zever/shared/redisopt v0.0.0 // indirect
	github.com/zenta-dev/zever/shared/registry v0.0.0 // indirect
)

replace (
	github.com/zenta-dev/zever/adapters/session/memory => ../../session/memory
	github.com/zenta-dev/zever/core/auth => ../../../core/auth
	github.com/zenta-dev/zever/core/session => ../../../core/session
)

replace github.com/zenta-dev/zever/shared/redisclient => ../../../shared/redisclient

replace github.com/zenta-dev/zever/shared/redisopt => ../../../shared/redisopt

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
