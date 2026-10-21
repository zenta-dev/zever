module github.com/zenta-dev/zever/adapters/secrets/vault

go 1.27.3

require github.com/zenta-dev/zever/core/secrets v0.8.0

require (
	github.com/zenta-dev/zever/shared/ref v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/registry v0.8.0 // indirect
)

replace github.com/zenta-dev/zever/core/secrets => ../../../core/secrets

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/adapters/secrets/env => ../env

replace github.com/zenta-dev/zever/shared/ref => ../../../shared/ref
