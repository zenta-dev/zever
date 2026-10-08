module github.com/zenta-dev/zever/adapters/document/local

go 1.27.0

require (
	github.com/chromedp/chromedp v0.19.1
	github.com/zenta-dev/zever/core/document v0.6.1
)

require (
	github.com/chromedp/cdproto v0.157.6 // indirect
	github.com/zenta-dev/zever/shared/endpoint v0.6.1 // indirect
	github.com/zenta-dev/zever/shared/registry v0.6.1 // indirect
)

replace github.com/zenta-dev/zever/core/document => ../../../core/document

replace github.com/zenta-dev/zever/shared/endpoint => ../../../shared/endpoint

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
