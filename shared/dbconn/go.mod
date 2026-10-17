module github.com/zenta-dev/zever/shared/dbconn

go 1.27.2

require github.com/zenta-dev/zever/core/db v0.7.2

require github.com/zenta-dev/zever/shared/registry v0.7.2 // indirect

replace github.com/zenta-dev/zever/core/db => ../../core/db

replace github.com/zenta-dev/zever/shared/registry => ../registry
