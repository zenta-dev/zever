module github.com/zenta-dev/zever/adapters/crypto/kms

go 1.27.0

require github.com/zenta-dev/zever/core/crypto v0.5.3

require github.com/zenta-dev/zever/shared/registry v0.5.3 // indirect

replace github.com/zenta-dev/zever/core/crypto => ../../../core/crypto

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
