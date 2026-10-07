module github.com/zenta-dev/zever/shared/dbconn

go 1.27.0

require github.com/zenta-dev/zever/core/db v0.6.0

require github.com/zenta-dev/zever/shared/registry v0.5.3 // indirect

replace github.com/zenta-dev/zever/core/db => ../../core/db
