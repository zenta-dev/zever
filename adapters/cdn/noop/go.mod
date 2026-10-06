module github.com/zenta-dev/zever/adapters/cdn/noop

go 1.27.0

require github.com/zenta-dev/zever/core/cdn v0.5.3

require github.com/zenta-dev/zever/shared/registry v0.5.3 // indirect

replace github.com/zenta-dev/zever/core/cdn => ../../../core/cdn
