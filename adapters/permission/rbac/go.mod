module github.com/zenta-dev/zever/adapters/permission/rbac

go 1.27.0

require github.com/zenta-dev/zever/core/permission v0.0.0

require github.com/zenta-dev/zever/shared/registry v0.0.0 // indirect

replace github.com/zenta-dev/zever/core/permission => ../../../core/permission

replace github.com/zenta-dev/zever/adapters/permission/noop => ../noop

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
