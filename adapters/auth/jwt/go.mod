module github.com/zenta-dev/zever/adapters/auth/jwt

go 1.27.0

require (
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/zenta-dev/zever/core/auth v0.0.0
)

require (
	github.com/zenta-dev/zever/core/session v0.0.0 // indirect
	github.com/zenta-dev/zever/shared/redisopt v0.0.0 // indirect
	github.com/zenta-dev/zever/shared/registry v0.0.0 // indirect
)

replace github.com/zenta-dev/zever/core/auth => ../../../core/auth

replace github.com/zenta-dev/zever/adapters/auth/session => ../session

replace github.com/zenta-dev/zever/adapters/session/memory => ../../session/memory

replace github.com/zenta-dev/zever/core/session => ../../../core/session

replace github.com/zenta-dev/zever/shared/redisclient => ../../../shared/redisclient

replace github.com/zenta-dev/zever/shared/redisopt => ../../../shared/redisopt

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
