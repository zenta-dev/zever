module github.com/zenta-dev/zever/adapters/tenant/single

go 1.27.0

require github.com/zenta-dev/zever/core/tenant v0.0.0

require github.com/zenta-dev/zever/shared/registry v0.0.0 // indirect

replace github.com/zenta-dev/zever/core/tenant => ../../../core/tenant

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
