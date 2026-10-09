module github.com/zenta-dev/zever/adapters/session/redis

go 1.27.2

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	github.com/zenta-dev/zever/core/db v0.6.1 // indirect
	github.com/zenta-dev/zever/shared/registry v0.6.1 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
)

replace github.com/zenta-dev/zever/shared/codec => ../../../shared/codec

replace github.com/zenta-dev/zever/shared/redisclient => ../../../shared/redisclient

replace github.com/zenta-dev/zever/shared/redisopt => ../../../shared/redisopt

require (
	github.com/alicebob/miniredis/v2 v2.39.0
	github.com/redis/go-redis/v9 v9.22.0
	github.com/zenta-dev/zever/core/session v0.6.1
	github.com/zenta-dev/zever/shared/codec v0.6.1
	github.com/zenta-dev/zever/shared/redisclient v0.6.1
	github.com/zenta-dev/zever/shared/redisopt v0.6.1
)

replace github.com/zenta-dev/zever/core/session => ../../../core/session

replace github.com/zenta-dev/zever/adapters/session/memory => ../memory

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/core/db => ../../../core/db
