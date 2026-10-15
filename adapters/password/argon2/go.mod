module github.com/zenta-dev/zever/adapters/password/argon2

go 1.27.2

require (
	github.com/zenta-dev/zever/shared/registry v0.7.1 // indirect
	golang.org/x/sys v0.49.0 // indirect
)

require (
	github.com/zenta-dev/zever/core/password v0.7.1
	golang.org/x/crypto v0.58.0
)

replace github.com/zenta-dev/zever/core/password => ../../../core/password

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
