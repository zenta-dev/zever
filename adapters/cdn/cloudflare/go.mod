module github.com/zenta-dev/zever/adapters/cdn/cloudflare

go 1.27.0

require (
	github.com/cloudflare/cloudflare-go/v7 v7.12.0
	github.com/zenta-dev/zever/core/cdn v0.6.0
)

require (
	github.com/tidwall/gjson v1.14.4 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	github.com/zenta-dev/zever/shared/registry v0.6.0 // indirect
)

replace github.com/zenta-dev/zever/core/cdn => ../../../core/cdn

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
