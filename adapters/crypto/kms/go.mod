module github.com/zenta-dev/zever/adapters/crypto/kms

go 1.27.2

require github.com/zenta-dev/zever/core/crypto v0.7.2

require (
	github.com/zenta-dev/zever/shared/ref v0.7.2 // indirect
	github.com/zenta-dev/zever/shared/registry v0.7.2 // indirect
)

replace github.com/zenta-dev/zever/core/crypto => ../../../core/crypto

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/adapters/crypto/local => ../local

replace github.com/zenta-dev/zever/shared/ref => ../../../shared/ref
