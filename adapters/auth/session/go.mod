module github.com/zenta-dev/zever/adapters/auth/session

go 1.27.2

require (
	github.com/zenta-dev/zever/adapters/session/memory v0.7.2
	github.com/zenta-dev/zever/core/auth v0.7.2
	github.com/zenta-dev/zever/core/session v0.7.2
)

require (
	github.com/zenta-dev/zever/core/db v0.7.2 // indirect
	github.com/zenta-dev/zever/shared/endpoint v0.7.2 // indirect
	github.com/zenta-dev/zever/shared/redisopt v0.7.2 // indirect
	github.com/zenta-dev/zever/shared/ref v0.7.2 // indirect
	github.com/zenta-dev/zever/shared/registry v0.7.2 // indirect
)

replace (
	github.com/zenta-dev/zever/adapters/session/memory => ../../session/memory
	github.com/zenta-dev/zever/core/auth => ../../../core/auth
	github.com/zenta-dev/zever/core/session => ../../../core/session
)

replace github.com/zenta-dev/zever/shared/redisclient => ../../../shared/redisclient

replace github.com/zenta-dev/zever/shared/redisopt => ../../../shared/redisopt

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/core/db => ../../../core/db

replace github.com/zenta-dev/zever/shared/endpoint => ../../../shared/endpoint

replace github.com/zenta-dev/zever/shared/ref => ../../../shared/ref
