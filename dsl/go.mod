module github.com/zenta-dev/zever/dsl

go 1.27.0

require (
	github.com/bufbuild/protocompile v0.14.1
	github.com/google/uuid v1.6.0
	github.com/robfig/cron/v3 v3.0.1
	google.golang.org/protobuf v1.36.12
)

require (
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	google.golang.org/grpc v1.83.2 // indirect
	google.golang.org/grpc/cmd/protoc-gen-go-grpc v1.6.2 // indirect
)

tool google.golang.org/grpc/cmd/protoc-gen-go-grpc

replace github.com/zenta-dev/zever/core/auth => ../core/auth

replace github.com/zenta-dev/zever/core/authz => ../core/authz

replace github.com/zenta-dev/zever/core/permission => ../core/permission

replace github.com/zenta-dev/zever/core/router => ../core/router

replace github.com/zenta-dev/zever/orm => ../orm

replace github.com/zenta-dev/zever/shared/apperror => ../shared/apperror

replace github.com/zenta-dev/zever/adapters/auth/session => ../adapters/auth/session

replace github.com/zenta-dev/zever/adapters/db/postgres => ../adapters/db/postgres

replace github.com/zenta-dev/zever/adapters/db/sqlite => ../adapters/db/sqlite

replace github.com/zenta-dev/zever/adapters/log/noop => ../adapters/log/noop

replace github.com/zenta-dev/zever/adapters/permission/noop => ../adapters/permission/noop

replace github.com/zenta-dev/zever/adapters/router/stdhttp => ../adapters/router/stdhttp

replace github.com/zenta-dev/zever/adapters/session/memory => ../adapters/session/memory

replace github.com/zenta-dev/zever/core/db => ../core/db

replace github.com/zenta-dev/zever/core/log => ../core/log

replace github.com/zenta-dev/zever/core/session => ../core/session

replace github.com/zenta-dev/zever/shared/codec => ../shared/codec

replace github.com/zenta-dev/zever/shared/lrucache => ../shared/lrucache

replace github.com/zenta-dev/zever/shared/redisclient => ../shared/redisclient

replace github.com/zenta-dev/zever/shared/redisopt => ../shared/redisopt

replace github.com/zenta-dev/zever/shared/registry => ../shared/registry

replace github.com/zenta-dev/zever/shared/retry => ../shared/retry
