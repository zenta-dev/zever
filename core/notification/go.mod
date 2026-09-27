module github.com/zenta-dev/zever/core/notification

go 1.27.0

require (
	github.com/zenta-dev/zever/adapters/notification/log v0.0.0
	github.com/zenta-dev/zever/shared/registry v0.0.0
)

require github.com/zenta-dev/zever/shared/codec v0.0.0 // indirect

replace github.com/zenta-dev/zever/shared/registry => ../../shared/registry

replace github.com/zenta-dev/zever/adapters/notification/log => ../../adapters/notification/log

replace github.com/zenta-dev/zever/shared/codec => ../../shared/codec
