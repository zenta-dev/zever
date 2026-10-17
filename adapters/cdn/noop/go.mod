module github.com/zenta-dev/zever/adapters/cdn/noop

go 1.27.2

require github.com/zenta-dev/zever/core/cdn v0.7.2

require github.com/zenta-dev/zever/shared/registry v0.7.2 // indirect

replace github.com/zenta-dev/zever/core/cdn => ../../../core/cdn

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
