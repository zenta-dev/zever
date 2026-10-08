module github.com/zenta-dev/zever/adapters/mailer/log

go 1.27.0

require (
	github.com/zenta-dev/zever/core/mailer v0.6.1
	github.com/zenta-dev/zever/shared/traceprop v0.6.1
	go.opentelemetry.io/otel/trace v1.47.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/zenta-dev/zever/shared/registry v0.6.1 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.47.0 // indirect
	go.opentelemetry.io/otel/log v1.47.0 // indirect
	go.opentelemetry.io/otel/metric v1.47.0 // indirect
)

replace (
	github.com/zenta-dev/zever/core/mailer => ../../../core/mailer
	github.com/zenta-dev/zever/shared/codec => ../../../shared/codec
)

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/shared/traceprop => ../../../shared/traceprop
