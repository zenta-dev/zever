module github.com/zenta-dev/zever/orm

go 1.27.0

require (
	github.com/jackc/pgx/v5 v5.11.0
	github.com/zenta-dev/zever/adapters/db/postgres v0.0.0
	github.com/zenta-dev/zever/adapters/db/sqlite v0.0.0
	github.com/zenta-dev/zever/core/db v0.0.0
	github.com/zenta-dev/zever/dsl v0.0.0
	github.com/zenta-dev/zever/shared/retry v0.0.0
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	github.com/zenta-dev/zever/shared/lrucache v0.0.0 // indirect
	github.com/zenta-dev/zever/shared/registry v0.0.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	modernc.org/libc v1.75.7 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
	modernc.org/sqlite v1.59.0 // indirect
)

replace (
	github.com/zenta-dev/zever/adapters/db/postgres => ../adapters/db/postgres
	github.com/zenta-dev/zever/adapters/db/sqlite => ../adapters/db/sqlite
	github.com/zenta-dev/zever/core/db => ../core/db
	github.com/zenta-dev/zever/dsl => ../dsl
	github.com/zenta-dev/zever/shared/lrucache => ../shared/lrucache
	github.com/zenta-dev/zever/shared/registry => ../shared/registry
	github.com/zenta-dev/zever/shared/retry => ../shared/retry
)

replace github.com/zenta-dev/zever/adapters/auth/session => ../adapters/auth/session

replace github.com/zenta-dev/zever/adapters/log/noop => ../adapters/log/noop

replace github.com/zenta-dev/zever/adapters/permission/noop => ../adapters/permission/noop

replace github.com/zenta-dev/zever/adapters/router/stdhttp => ../adapters/router/stdhttp

replace github.com/zenta-dev/zever/adapters/session/memory => ../adapters/session/memory

replace github.com/zenta-dev/zever/core/auth => ../core/auth

replace github.com/zenta-dev/zever/core/authz => ../core/authz

replace github.com/zenta-dev/zever/core/log => ../core/log

replace github.com/zenta-dev/zever/core/permission => ../core/permission

replace github.com/zenta-dev/zever/core/router => ../core/router

replace github.com/zenta-dev/zever/core/session => ../core/session

replace github.com/zenta-dev/zever/shared/apperror => ../shared/apperror

replace github.com/zenta-dev/zever/shared/codec => ../shared/codec

replace github.com/zenta-dev/zever/shared/redisclient => ../shared/redisclient

replace github.com/zenta-dev/zever/shared/redisopt => ../shared/redisopt
