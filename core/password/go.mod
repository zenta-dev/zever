module github.com/zenta-dev/zever/core/password

go 1.27.2

require (
	github.com/zenta-dev/zever/adapters/password/argon2 v0.7.2
	github.com/zenta-dev/zever/shared/registry v0.7.2
)

require (
	golang.org/x/crypto v0.58.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
)

replace github.com/zenta-dev/zever/shared/registry => ../../shared/registry

replace github.com/zenta-dev/zever/adapters/password/argon2 => ../../adapters/password/argon2
