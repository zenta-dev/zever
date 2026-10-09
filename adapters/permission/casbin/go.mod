module github.com/zenta-dev/zever/adapters/permission/casbin

go 1.27.2

require (
	github.com/bmatcuk/doublestar/v4 v4.6.1 // indirect
	github.com/casbin/govaluate v1.3.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/zenta-dev/zever/shared/registry v0.6.1 // indirect
)

require (
	github.com/casbin/casbin/v2 v2.135.0
	github.com/zenta-dev/zever/core/permission v0.6.1
)

replace github.com/zenta-dev/zever/core/permission => ../../../core/permission

replace github.com/zenta-dev/zever/adapters/permission/noop => ../noop

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
