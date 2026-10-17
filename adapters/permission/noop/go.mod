module github.com/zenta-dev/zever/adapters/permission/noop

go 1.27.2

require github.com/zenta-dev/zever/core/permission v0.7.2

require github.com/zenta-dev/zever/shared/registry v0.7.2 // indirect

replace github.com/zenta-dev/zever/core/permission => ../../../core/permission

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
