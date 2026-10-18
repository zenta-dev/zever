module github.com/zenta-dev/zever/adapters/lock/redis

go 1.27.2

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	github.com/zenta-dev/zever/shared/ref v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/registry v0.8.0 // indirect
	go.uber.org/atomic v1.12.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
)

replace github.com/zenta-dev/zever/shared/redisclient => ../../../shared/redisclient

replace github.com/zenta-dev/zever/shared/redisopt => ../../../shared/redisopt

require (
	github.com/alicebob/miniredis/v2 v2.40.0
	github.com/redis/go-redis/v9 v9.23.0
	github.com/zenta-dev/zever/core/lock v0.8.0
	github.com/zenta-dev/zever/shared/cas v0.8.0
	github.com/zenta-dev/zever/shared/redisclient v0.8.0
	github.com/zenta-dev/zever/shared/redisopt v0.8.0
)

replace github.com/zenta-dev/zever/core/lock => ../../../core/lock

replace github.com/zenta-dev/zever/shared/cas => ../../../shared/cas

replace github.com/zenta-dev/zever/adapters/lock/memory => ../memory

replace github.com/zenta-dev/zever/shared/lrucache => ../../../shared/lrucache

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/shared/ref => ../../../shared/ref
