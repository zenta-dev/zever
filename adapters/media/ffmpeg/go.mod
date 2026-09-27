module github.com/zenta-dev/zever/adapters/media/ffmpeg

go 1.27.0

require (
	github.com/zenta-dev/zever/core/media v0.0.0
	github.com/zenta-dev/zever/shared/codec v0.0.0
)

require github.com/zenta-dev/zever/shared/registry v0.0.0 // indirect

replace (
	github.com/zenta-dev/zever/core/media => ../../../core/media
	github.com/zenta-dev/zever/shared/codec => ../../../shared/codec
)

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
