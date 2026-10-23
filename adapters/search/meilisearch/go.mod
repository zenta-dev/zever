module github.com/zenta-dev/zever/adapters/search/meilisearch

go 1.27.3

require (
	github.com/andybalholm/brotli v1.1.1 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/zenta-dev/zever/core/db v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/endpoint v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/ref v0.8.0 // indirect
)

require (
	github.com/meilisearch/meilisearch-go v0.36.3
	github.com/zenta-dev/zever/core/search v0.8.0
	github.com/zenta-dev/zever/shared/codec v0.8.0
	github.com/zenta-dev/zever/shared/lrucache v0.8.0
)

replace github.com/zenta-dev/zever/core/search => ../../../core/search

replace github.com/zenta-dev/zever/shared/codec => ../../../shared/codec

replace github.com/zenta-dev/zever/shared/lrucache => ../../../shared/lrucache

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/core/db => ../../../core/db

replace github.com/zenta-dev/zever/shared/endpoint => ../../../shared/endpoint

replace github.com/zenta-dev/zever/shared/ref => ../../../shared/ref
