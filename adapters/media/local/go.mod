module github.com/zenta-dev/zever/adapters/media/local

go 1.27.0

require (
	github.com/HugoSmits86/nativewebp v1.3.0 // indirect
	github.com/zenta-dev/zever/shared/codec v0.5.2 // indirect
	github.com/zenta-dev/zever/shared/registry v0.5.2 // indirect
	golang.org/x/image v0.45.0 // indirect
)

require (
	github.com/anthonynsimon/bild v0.17.1
	github.com/zenta-dev/zever/adapters/media/ffmpeg v0.5.2
	github.com/zenta-dev/zever/core/media v0.5.2
)

replace github.com/zenta-dev/zever/core/media => ../../../core/media

replace github.com/zenta-dev/zever/adapters/media/ffmpeg => ../ffmpeg

replace github.com/zenta-dev/zever/shared/codec => ../../../shared/codec

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
