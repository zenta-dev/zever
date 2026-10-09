module github.com/zenta-dev/zever/adapters/analytics/log

go 1.27.2

require github.com/zenta-dev/zever/core/analytics v0.6.1

require github.com/zenta-dev/zever/shared/registry v0.6.1 // indirect

replace github.com/zenta-dev/zever/core/analytics => ../../../core/analytics

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
