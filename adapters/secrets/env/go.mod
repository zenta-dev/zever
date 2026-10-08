module github.com/zenta-dev/zever/adapters/secrets/env

go 1.27.0

require github.com/zenta-dev/zever/core/secrets v0.6.1

require github.com/zenta-dev/zever/shared/registry v0.6.1 // indirect

replace github.com/zenta-dev/zever/core/secrets => ../../../core/secrets

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
