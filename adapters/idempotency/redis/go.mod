module github.com/zenta-dev/zever/adapters/idempotency/redis

go 1.27.2

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	github.com/zenta-dev/zever/core/db v0.7.2 // indirect
	github.com/zenta-dev/zever/shared/ref v0.7.2 // indirect
	github.com/zenta-dev/zever/shared/registry v0.7.2 // indirect
	go.uber.org/atomic v1.12.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
)

replace github.com/zenta-dev/zever/shared/redisclient => ../../../shared/redisclient

replace github.com/zenta-dev/zever/shared/redisopt => ../../../shared/redisopt

require (
	github.com/alicebob/miniredis/v2 v2.40.0
	github.com/redis/go-redis/v9 v9.23.0
	github.com/zenta-dev/zever/core/idempotency v0.7.2
	github.com/zenta-dev/zever/shared/redisclient v0.7.2
	github.com/zenta-dev/zever/shared/redisopt v0.7.2
)

replace github.com/zenta-dev/zever/core/idempotency => ../../../core/idempotency

replace github.com/zenta-dev/zever/adapters/idempotency/memory => ../memory

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/core/db => ../../../core/db

replace github.com/zenta-dev/zever/shared/ref => ../../../shared/ref
