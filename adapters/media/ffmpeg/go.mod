module github.com/zenta-dev/zever/adapters/media/ffmpeg

go 1.27.0

require (
	github.com/zenta-dev/zever/core/media v0.6.1
	github.com/zenta-dev/zever/shared/codec v0.6.1
)

require github.com/zenta-dev/zever/shared/registry v0.6.1 // indirect

replace (
	github.com/zenta-dev/zever/core/media => ../../../core/media
	github.com/zenta-dev/zever/shared/codec => ../../../shared/codec
)

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
