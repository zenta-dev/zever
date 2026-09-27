module github.com/zenta-dev/zever/adapters/notification/twilio

go 1.27.0

require (
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/golang/mock v1.6.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/zenta-dev/zever/shared/registry v0.0.0 // indirect
)

require (
	github.com/twilio/twilio-go v1.31.1
	github.com/zenta-dev/zever/core/notification v0.0.0
	github.com/zenta-dev/zever/shared/httpclient v0.0.0
)

replace github.com/zenta-dev/zever/core/notification => ../../../core/notification

replace github.com/zenta-dev/zever/shared/httpclient => ../../../shared/httpclient

replace github.com/zenta-dev/zever/adapters/notification/log => ../log

replace github.com/zenta-dev/zever/shared/codec => ../../../shared/codec

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
