module github.com/zenta-dev/zever/shared/dbconn

go 1.27.2

require github.com/zenta-dev/zever/core/db v0.8.0

require (
	github.com/zenta-dev/zever/shared/ref v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/registry v0.8.0 // indirect
)

replace github.com/zenta-dev/zever/core/db => ../../core/db

replace github.com/zenta-dev/zever/shared/registry => ../registry

replace github.com/zenta-dev/zever/shared/ref => ../../shared/ref
