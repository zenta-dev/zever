module github.com/zenta-dev/zever/adapters/outbox/cdc

go 1.27.0

require (
	github.com/jackc/pglogrepl v0.0.0-20261003132456-662581bb6bb6
	github.com/jackc/pgx/v5 v5.11.0
	github.com/zenta-dev/zever/core/db v0.5.3
	github.com/zenta-dev/zever/core/observability v0.5.3
	github.com/zenta-dev/zever/core/outbox v0.5.3
	github.com/zenta-dev/zever/shared/retry v0.5.3
)

require (
	github.com/jackc/pgio v1.0.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/zenta-dev/zever/shared/registry v0.5.3 // indirect
	golang.org/x/text v0.35.0 // indirect
)

replace github.com/zenta-dev/zever/core/db => ../../../core/db

replace github.com/zenta-dev/zever/core/observability => ../../../core/observability

replace github.com/zenta-dev/zever/core/outbox => ../../../core/outbox

replace github.com/zenta-dev/zever/shared/retry => ../../../shared/retry
