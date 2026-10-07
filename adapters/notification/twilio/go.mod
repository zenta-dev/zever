module github.com/zenta-dev/zever/adapters/notification/twilio

go 1.27.0

require (
	github.com/twilio/twilio-go v1.31.2
	github.com/zenta-dev/zever/core/notification v0.5.3
	github.com/zenta-dev/zever/shared/httpclient v0.5.3
)

replace github.com/zenta-dev/zever/core/notification => ../../../core/notification

replace github.com/zenta-dev/zever/shared/httpclient => ../../../shared/httpclient

replace github.com/zenta-dev/zever/adapters/notification/log => ../log

replace github.com/zenta-dev/zever/shared/codec => ../../../shared/codec

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/shared/traceprop => ../../../shared/traceprop

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/golang/mock v1.6.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/zenta-dev/zever/shared/registry v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/traceprop v0.5.3 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.47.0 // indirect
	go.opentelemetry.io/otel/log v1.47.0 // indirect
	go.opentelemetry.io/otel/metric v1.47.0 // indirect
	go.opentelemetry.io/otel/trace v1.47.0 // indirect
)
