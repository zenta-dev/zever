module github.com/zenta-dev/zever/core/analytics

go 1.27.2

require (
	github.com/zenta-dev/zever/adapters/analytics/log v0.7.2
	github.com/zenta-dev/zever/shared/ref v0.7.2
	github.com/zenta-dev/zever/shared/registry v0.7.2
)

replace github.com/zenta-dev/zever/shared/registry => ../../shared/registry

replace github.com/zenta-dev/zever/adapters/analytics/log => ../../adapters/analytics/log

replace github.com/zenta-dev/zever/shared/ref => ../../shared/ref
