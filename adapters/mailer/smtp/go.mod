module github.com/zenta-dev/zever/adapters/mailer/smtp

go 1.27.2

require github.com/zenta-dev/zever/core/mailer v0.8.0

replace github.com/zenta-dev/zever/core/mailer => ../../../core/mailer

replace github.com/zenta-dev/zever/adapters/mailer/log => ../log

replace github.com/zenta-dev/zever/shared/codec => ../../../shared/codec

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/shared/traceprop => ../../../shared/traceprop

require (
	github.com/zenta-dev/zever/shared/ref v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/registry v0.8.0 // indirect
)

replace github.com/zenta-dev/zever/shared/ref => ../../../shared/ref
