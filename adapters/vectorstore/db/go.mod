module github.com/zenta-dev/zever/adapters/vectorstore/db

go 1.27.0

require (
	github.com/zenta-dev/zever/adapters/db/postgres v0.6.0
	github.com/zenta-dev/zever/adapters/db/sqlite v0.6.0
	github.com/zenta-dev/zever/core/db v0.6.0
	github.com/zenta-dev/zever/core/vectorstore v0.6.0
	github.com/zenta-dev/zever/orm v0.6.0
	github.com/zenta-dev/zever/shared/codec v0.6.0
	github.com/zenta-dev/zever/shared/dbconn v0.6.0
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.11.0 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/zenta-dev/zever/shared/endpoint v0.6.0 // indirect
	github.com/zenta-dev/zever/shared/lrucache v0.6.0 // indirect
	github.com/zenta-dev/zever/shared/registry v0.6.0 // indirect
	github.com/zenta-dev/zever/shared/retry v0.6.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	modernc.org/libc v1.77.1 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
	modernc.org/sqlite v1.60.1 // indirect
)

replace github.com/zenta-dev/zever/adapters/db/postgres => ../../db/postgres

replace github.com/zenta-dev/zever/adapters/db/sqlite => ../../db/sqlite

replace github.com/zenta-dev/zever/core/db => ../../../core/db

replace github.com/zenta-dev/zever/core/vectorstore => ../../../core/vectorstore

replace github.com/zenta-dev/zever/orm => ../../../orm

replace github.com/zenta-dev/zever/shared/codec => ../../../shared/codec

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/shared/dbconn => ../../../shared/dbconn

replace github.com/zenta-dev/zever/shared/endpoint => ../../../shared/endpoint

replace github.com/zenta-dev/zever/shared/lrucache => ../../../shared/lrucache

replace github.com/zenta-dev/zever/shared/retry => ../../../shared/retry
