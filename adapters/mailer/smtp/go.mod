module github.com/zenta-dev/zever/adapters/mailer/smtp

go 1.27.0

require github.com/zenta-dev/zever/core/mailer v0.0.0

require github.com/zenta-dev/zever/shared/registry v0.0.0 // indirect

replace github.com/zenta-dev/zever/core/mailer => ../../../core/mailer

replace github.com/zenta-dev/zever/adapters/mailer/log => ../log

replace github.com/zenta-dev/zever/shared/codec => ../../../shared/codec

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
