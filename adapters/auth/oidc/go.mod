module github.com/zenta-dev/zever/adapters/auth/oidc

go 1.27.0

require (
	github.com/coreos/go-oidc/v3 v3.21.0
	github.com/zenta-dev/zever/core/auth v0.5.3
	github.com/zenta-dev/zever/shared/httpclient v0.5.3
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-jose/go-jose/v4 v4.1.4 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/zenta-dev/zever/core/db v0.5.3 // indirect
	github.com/zenta-dev/zever/core/session v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/endpoint v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/redisopt v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/registry v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/traceprop v0.5.3 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.47.0 // indirect
	go.opentelemetry.io/otel/log v1.47.0 // indirect
	go.opentelemetry.io/otel/metric v1.47.0 // indirect
	go.opentelemetry.io/otel/trace v1.47.0 // indirect
	golang.org/x/oauth2 v0.36.0 // indirect
)

replace github.com/zenta-dev/zever/core/auth => ../../../core/auth

replace github.com/zenta-dev/zever/shared/httpclient => ../../../shared/httpclient

replace github.com/zenta-dev/zever/adapters/auth/session => ../session

replace github.com/zenta-dev/zever/adapters/session/memory => ../../session/memory

replace github.com/zenta-dev/zever/core/session => ../../../core/session

replace github.com/zenta-dev/zever/shared/redisclient => ../../../shared/redisclient

replace github.com/zenta-dev/zever/shared/redisopt => ../../../shared/redisopt

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/core/db => ../../../core/db

replace github.com/zenta-dev/zever/shared/endpoint => ../../../shared/endpoint

replace github.com/zenta-dev/zever/shared/traceprop => ../../../shared/traceprop
