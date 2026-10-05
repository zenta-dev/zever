module github.com/zenta-dev/zever/core/mailer

go 1.27.0

require (
	github.com/zenta-dev/zever/adapters/mailer/log v0.5.3
	github.com/zenta-dev/zever/shared/registry v0.5.3
)

replace github.com/zenta-dev/zever/shared/registry => ../../shared/registry

replace github.com/zenta-dev/zever/adapters/mailer/log => ../../adapters/mailer/log

replace github.com/zenta-dev/zever/shared/codec => ../../shared/codec

replace github.com/zenta-dev/zever/shared/traceprop => ../../shared/traceprop

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/zenta-dev/zever/shared/traceprop v0.5.3 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.46.0 // indirect
	go.opentelemetry.io/otel/metric v1.46.0 // indirect
	go.opentelemetry.io/otel/trace v1.46.0 // indirect
)
