module github.com/zenta-dev/zever/core/analytics

go 1.27.3

require (
	github.com/zenta-dev/zever/adapters/analytics/log v0.8.0
	github.com/zenta-dev/zever/shared/codec v0.8.0
	github.com/zenta-dev/zever/shared/ref v0.8.0
)

replace github.com/zenta-dev/zever/shared/registry => ../../shared/registry

replace github.com/zenta-dev/zever/adapters/analytics/log => ../../adapters/analytics/log

replace github.com/zenta-dev/zever/shared/ref => ../../shared/ref

replace github.com/zenta-dev/zever/shared/codec => ../../shared/codec
