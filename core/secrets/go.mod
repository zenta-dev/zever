module github.com/zenta-dev/zever/core/secrets

go 1.27.3

require (
	github.com/zenta-dev/zever/adapters/secrets/env v0.8.0
	github.com/zenta-dev/zever/shared/ref v0.8.0
	github.com/zenta-dev/zever/shared/registry v0.8.0
)

replace github.com/zenta-dev/zever/shared/registry => ../../shared/registry

replace github.com/zenta-dev/zever/adapters/secrets/env => ../../adapters/secrets/env

replace github.com/zenta-dev/zever/shared/ref => ../../shared/ref
