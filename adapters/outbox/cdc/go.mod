module github.com/zenta-dev/zever/adapters/outbox/cdc

go 1.27.0

require (
	github.com/jackc/pglogrepl v0.0.0-20261003132456-662581bb6bb6
	github.com/jackc/pgx/v5 v5.11.0
	github.com/zenta-dev/zever/core/db v0.6.1
	github.com/zenta-dev/zever/core/observability v0.6.1
	github.com/zenta-dev/zever/core/outbox v0.6.1
	github.com/zenta-dev/zever/shared/retry v0.6.1
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/jackc/pgio v1.0.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/zenta-dev/zever/shared/registry v0.6.1 // indirect
	github.com/zenta-dev/zever/shared/traceprop v0.6.1 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.47.0 // indirect
	go.opentelemetry.io/otel/log v1.47.0 // indirect
	go.opentelemetry.io/otel/metric v1.47.0 // indirect
	go.opentelemetry.io/otel/trace v1.47.0 // indirect
	golang.org/x/text v0.35.0 // indirect
)

replace github.com/zenta-dev/zever/core/db => ../../../core/db

replace github.com/zenta-dev/zever/core/observability => ../../../core/observability

replace github.com/zenta-dev/zever/core/outbox => ../../../core/outbox

replace github.com/zenta-dev/zever/shared/retry => ../../../shared/retry

replace github.com/zenta-dev/zever/shared/traceprop => ../../../shared/traceprop

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/adapters/observability/noop => ../../observability/noop
