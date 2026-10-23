module github.com/zenta-dev/zever/core/auth

go 1.27.3

require (
	github.com/alicebob/miniredis/v2 v2.40.0
	github.com/redis/go-redis/v9 v9.23.0
	github.com/zenta-dev/zever/core/session v0.8.0
	github.com/zenta-dev/zever/shared/codec v0.8.0
	github.com/zenta-dev/zever/shared/endpoint v0.8.0
	github.com/zenta-dev/zever/shared/redisclient v0.8.0
	github.com/zenta-dev/zever/shared/redisopt v0.8.0
	github.com/zenta-dev/zever/shared/ref v0.8.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	github.com/zenta-dev/zever/core/db v0.8.0 // indirect
	go.uber.org/atomic v1.12.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
)

replace (
	github.com/zenta-dev/zever/adapters/auth/session => ../../adapters/auth/session
	github.com/zenta-dev/zever/core/session => ../session
	github.com/zenta-dev/zever/shared/endpoint => ../../shared/endpoint
	github.com/zenta-dev/zever/shared/redisclient => ../../shared/redisclient
	github.com/zenta-dev/zever/shared/registry => ../../shared/registry
)

replace github.com/zenta-dev/zever/shared/redisopt => ../../shared/redisopt

replace github.com/zenta-dev/zever/adapters/session/memory => ../../adapters/session/memory

replace github.com/zenta-dev/zever/core/db => ../db

replace github.com/zenta-dev/zever/shared/ref => ../../shared/ref

replace github.com/zenta-dev/zever/shared/codec => ../../shared/codec
