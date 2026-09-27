module github.com/zenta-dev/zever/tools/zever-lsp

go 1.27.0

require (
	github.com/zenta-dev/zever/dsl v0.0.0
	go.lsp.dev/jsonrpc2 v1.0.1
	go.lsp.dev/protocol v1.0.1
	go.lsp.dev/uri v1.0.1
)

require (
	github.com/go-json-experiment/json v0.0.0-20260623181947-01eb4420fa68 // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
)

replace github.com/zenta-dev/zever/dsl => ../../dsl

replace github.com/zenta-dev/zever/adapters/auth/session => ../../adapters/auth/session

replace github.com/zenta-dev/zever/adapters/db/postgres => ../../adapters/db/postgres

replace github.com/zenta-dev/zever/adapters/db/sqlite => ../../adapters/db/sqlite

replace github.com/zenta-dev/zever/adapters/log/noop => ../../adapters/log/noop

replace github.com/zenta-dev/zever/adapters/permission/noop => ../../adapters/permission/noop

replace github.com/zenta-dev/zever/adapters/router/stdhttp => ../../adapters/router/stdhttp

replace github.com/zenta-dev/zever/adapters/session/memory => ../../adapters/session/memory

replace github.com/zenta-dev/zever/core/auth => ../../core/auth

replace github.com/zenta-dev/zever/core/authz => ../../core/authz

replace github.com/zenta-dev/zever/core/db => ../../core/db

replace github.com/zenta-dev/zever/core/log => ../../core/log

replace github.com/zenta-dev/zever/core/permission => ../../core/permission

replace github.com/zenta-dev/zever/core/router => ../../core/router

replace github.com/zenta-dev/zever/core/session => ../../core/session

replace github.com/zenta-dev/zever/orm => ../../orm

replace github.com/zenta-dev/zever/shared/apperror => ../../shared/apperror

replace github.com/zenta-dev/zever/shared/codec => ../../shared/codec

replace github.com/zenta-dev/zever/shared/lrucache => ../../shared/lrucache

replace github.com/zenta-dev/zever/shared/redisclient => ../../shared/redisclient

replace github.com/zenta-dev/zever/shared/redisopt => ../../shared/redisopt

replace github.com/zenta-dev/zever/shared/registry => ../../shared/registry

replace github.com/zenta-dev/zever/shared/retry => ../../shared/retry
