module github.com/zenta-dev/zever/core/i18n

go 1.27.0

require (
	github.com/zenta-dev/zever/adapters/i18n/embed v0.0.0
	github.com/zenta-dev/zever/shared/endpoint v0.0.0
	github.com/zenta-dev/zever/shared/registry v0.0.0
)

require github.com/zenta-dev/zever/shared/codec v0.0.0 // indirect

replace (
	github.com/zenta-dev/zever/shared/endpoint => ../../shared/endpoint
	github.com/zenta-dev/zever/shared/registry => ../../shared/registry
)

replace github.com/zenta-dev/zever/adapters/i18n/embed => ../../adapters/i18n/embed

replace github.com/zenta-dev/zever/shared/codec => ../../shared/codec
