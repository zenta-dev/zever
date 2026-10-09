module github.com/zenta-dev/zever/adapters/i18n/remote

go 1.27.2

require (
	github.com/zenta-dev/zever/core/i18n v0.6.1
	github.com/zenta-dev/zever/shared/codec v0.6.1
	github.com/zenta-dev/zever/shared/httpclient v0.6.1
	github.com/zenta-dev/zever/shared/lrucache v0.6.1
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/zenta-dev/zever/shared/endpoint v0.6.1 // indirect
	github.com/zenta-dev/zever/shared/registry v0.6.1 // indirect
	github.com/zenta-dev/zever/shared/traceprop v0.6.1 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.47.0 // indirect
	go.opentelemetry.io/otel/log v1.47.0 // indirect
	go.opentelemetry.io/otel/metric v1.47.0 // indirect
	go.opentelemetry.io/otel/trace v1.47.0 // indirect
)

replace (
	github.com/zenta-dev/zever/core/i18n => ../../../core/i18n
	github.com/zenta-dev/zever/shared/codec => ../../../shared/codec
	github.com/zenta-dev/zever/shared/httpclient => ../../../shared/httpclient
	github.com/zenta-dev/zever/shared/lrucache => ../../../shared/lrucache
)

replace github.com/zenta-dev/zever/adapters/i18n/embed => ../embed

replace github.com/zenta-dev/zever/shared/endpoint => ../../../shared/endpoint

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/shared/traceprop => ../../../shared/traceprop
