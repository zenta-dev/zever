module github.com/zenta-dev/zever/adapters/resilience/redis

go 1.27.2

require (
	github.com/alicebob/miniredis/v2 v2.39.0
	github.com/sony/gobreaker/v2 v2.4.0
	github.com/sony/gobreaker/v2/redis v0.0.0-20260207092134-fed8e9eb35f9
	github.com/zenta-dev/zever/core/resilience v0.6.1
	github.com/zenta-dev/zever/shared/redisclient v0.6.1
	github.com/zenta-dev/zever/shared/redisopt v0.6.1
	github.com/zenta-dev/zever/shared/retry v0.6.1
	golang.org/x/sync v0.23.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-redsync/redsync/v4 v4.13.0 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/redis/go-redis/v9 v9.22.0 // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	github.com/zenta-dev/zever/shared/registry v0.6.1 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/sys v0.30.0 // indirect
)

replace github.com/zenta-dev/zever/core/resilience => ../../../core/resilience

replace github.com/zenta-dev/zever/shared/redisclient => ../../../shared/redisclient

replace github.com/zenta-dev/zever/shared/redisopt => ../../../shared/redisopt

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/shared/retry => ../../../shared/retry
