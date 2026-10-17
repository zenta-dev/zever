module github.com/zenta-dev/zever/adapters/document/local

go 1.27.2

require (
	github.com/chromedp/chromedp v0.21.0
	github.com/zenta-dev/zever/core/document v0.7.2
)

require (
	github.com/chromedp/cdproto v0.157.8 // indirect
	github.com/go-json-experiment/json v0.0.0-20260213210345-44df1a37e875 // indirect
	github.com/zenta-dev/zever/shared/endpoint v0.7.2 // indirect
	github.com/zenta-dev/zever/shared/registry v0.7.2 // indirect
)

replace github.com/zenta-dev/zever/core/document => ../../../core/document

replace github.com/zenta-dev/zever/shared/endpoint => ../../../shared/endpoint

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
