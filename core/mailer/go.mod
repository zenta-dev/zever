module github.com/zenta-dev/zever/core/mailer

go 1.27.0

require (
	github.com/zenta-dev/zever/adapters/mailer/log v0.5.2
	github.com/zenta-dev/zever/shared/registry v0.5.2
)

require github.com/zenta-dev/zever/shared/codec v0.5.2 // indirect

replace github.com/zenta-dev/zever/shared/registry => ../../shared/registry

replace github.com/zenta-dev/zever/adapters/mailer/log => ../../adapters/mailer/log

replace github.com/zenta-dev/zever/shared/codec => ../../shared/codec
