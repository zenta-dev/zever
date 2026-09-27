module github.com/zenta-dev/zever/core/auth

go 1.27.0

require (
	github.com/alicebob/miniredis/v2 v2.39.0
	github.com/redis/go-redis/v9 v9.22.0
	github.com/zenta-dev/zever/core/session v0.0.0
	github.com/zenta-dev/zever/shared/redisclient v0.0.0
	github.com/zenta-dev/zever/shared/redisopt v0.0.0
	github.com/zenta-dev/zever/shared/registry v0.0.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/sys v0.30.0 // indirect
)

replace (
	github.com/zenta-dev/zever/adapters/auth/session => ../../adapters/auth/session
	github.com/zenta-dev/zever/core/session => ../session
	github.com/zenta-dev/zever/shared/redisclient => ../../shared/redisclient
	github.com/zenta-dev/zever/shared/registry => ../../shared/registry
)

replace github.com/zenta-dev/zever/shared/redisopt => ../../shared/redisopt

replace github.com/zenta-dev/zever/adapters/session/memory => ../../adapters/session/memory
