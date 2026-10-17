module github.com/zenta-dev/zever/adapters/secrets/vault

go 1.27.2

require github.com/zenta-dev/zever/core/secrets v0.7.2

require (
	github.com/zenta-dev/zever/shared/ref v0.7.2 // indirect
	github.com/zenta-dev/zever/shared/registry v0.7.2 // indirect
)

replace github.com/zenta-dev/zever/core/secrets => ../../../core/secrets

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/adapters/secrets/env => ../env

replace github.com/zenta-dev/zever/shared/ref => ../../../shared/ref
