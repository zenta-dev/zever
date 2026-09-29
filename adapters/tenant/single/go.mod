module github.com/zenta-dev/zever/adapters/tenant/single

go 1.27.0

require github.com/zenta-dev/zever/core/tenant v0.5.3

require github.com/zenta-dev/zever/shared/registry v0.5.3 // indirect

replace github.com/zenta-dev/zever/core/tenant => ../../../core/tenant

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
