module github.com/zenta-dev/zever/adapters/crypto/local

go 1.27.0

require github.com/zenta-dev/zever/core/crypto v0.0.0

require github.com/zenta-dev/zever/shared/registry v0.0.0 // indirect

replace github.com/zenta-dev/zever/core/crypto => ../../../core/crypto

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
