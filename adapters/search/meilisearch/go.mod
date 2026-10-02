module github.com/zenta-dev/zever/adapters/search/meilisearch

go 1.27.0

require (
	github.com/andybalholm/brotli v1.1.1 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/zenta-dev/zever/core/db v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/registry v0.5.3 // indirect
)

require (
	github.com/meilisearch/meilisearch-go v0.36.3
	github.com/zenta-dev/zever/core/search v0.5.3
	github.com/zenta-dev/zever/shared/codec v0.5.3
	github.com/zenta-dev/zever/shared/lrucache v0.5.3
)

replace github.com/zenta-dev/zever/core/search => ../../../core/search

replace github.com/zenta-dev/zever/shared/codec => ../../../shared/codec

replace github.com/zenta-dev/zever/shared/lrucache => ../../../shared/lrucache

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/core/db => ../../../core/db
