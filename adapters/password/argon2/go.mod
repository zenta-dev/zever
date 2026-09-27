module github.com/zenta-dev/zever/adapters/password/argon2

go 1.27.0

require (
	github.com/zenta-dev/zever/shared/registry v0.0.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
)

require (
	github.com/zenta-dev/zever/core/password v0.0.0
	golang.org/x/crypto v0.57.0
)

replace github.com/zenta-dev/zever/core/password => ../../../core/password

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
