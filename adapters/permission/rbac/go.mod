module github.com/zenta-dev/zever/adapters/permission/rbac

go 1.27.3

require github.com/zenta-dev/zever/core/permission v0.8.0

require github.com/zenta-dev/zever/shared/ref v0.8.0 // indirect

replace github.com/zenta-dev/zever/core/permission => ../../../core/permission

replace github.com/zenta-dev/zever/adapters/permission/noop => ../noop

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/shared/ref => ../../../shared/ref
