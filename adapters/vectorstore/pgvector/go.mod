module github.com/zenta-dev/zever/adapters/vectorstore/pgvector

go 1.27.0

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/zenta-dev/zever/shared/registry v0.0.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/text v0.42.0 // indirect
)

require (
	github.com/jackc/pgx/v5 v5.11.0
	github.com/zenta-dev/zever/core/vectorstore v0.0.0
	github.com/zenta-dev/zever/shared/codec v0.0.0
)

replace github.com/zenta-dev/zever/core/vectorstore => ../../../core/vectorstore

replace github.com/zenta-dev/zever/shared/codec => ../../../shared/codec

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
