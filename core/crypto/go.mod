module github.com/zenta-dev/zever/core/crypto

go 1.27.2

require (
	github.com/zenta-dev/zever/adapters/crypto/local v0.6.1
	github.com/zenta-dev/zever/shared/registry v0.6.1
)

replace (
	github.com/zenta-dev/zever/adapters/crypto/local => ../../adapters/crypto/local
	github.com/zenta-dev/zever/shared/registry => ../../shared/registry
)
