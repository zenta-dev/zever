module github.com/zenta-dev/zever/tools/zever-mcp

go 1.27.3

require github.com/zenta-dev/zever/dsl v0.8.0

require (
	github.com/robfig/cron/v3 v3.0.1 // indirect
	github.com/zenta-dev/zever/shared/codec v0.8.0 // indirect
)

replace github.com/zenta-dev/zever/dsl => ../../dsl

replace github.com/zenta-dev/zever/shared/codec => ../../shared/codec

replace github.com/zenta-dev/zever/shared/ref => ../../shared/ref
