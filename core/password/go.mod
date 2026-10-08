module github.com/zenta-dev/zever/core/password

go 1.27.0

require (
	github.com/zenta-dev/zever/adapters/password/argon2 v0.6.0
	github.com/zenta-dev/zever/shared/registry v0.6.0
)

require (
	golang.org/x/crypto v0.57.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
)

replace github.com/zenta-dev/zever/shared/registry => ../../shared/registry

replace github.com/zenta-dev/zever/adapters/password/argon2 => ../../adapters/password/argon2
