module github.com/zenta-dev/zever/adapters/analytics/posthog

go 1.27.0

require (
	github.com/posthog/posthog-go v1.25.2
	github.com/zenta-dev/zever/core/analytics v0.0.0
)

require (
	github.com/andybalholm/brotli v1.1.1 // indirect
	github.com/goccy/go-json v0.10.6 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/klauspost/compress v1.20.0 // indirect
	github.com/zenta-dev/zever/shared/registry v0.0.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
)

replace github.com/zenta-dev/zever/core/analytics => ../../../core/analytics

replace github.com/zenta-dev/zever/adapters/analytics/log => ../log

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
