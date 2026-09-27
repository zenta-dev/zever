module github.com/zenta-dev/zever/examples

go 1.27.0

require (
	github.com/zenta-dev/zever/adapters/db/sqlite v0.0.0
	github.com/zenta-dev/zever/config v0.0.0
	github.com/zenta-dev/zever/container v0.0.0
	github.com/zenta-dev/zever/core/db v0.0.0
	github.com/zenta-dev/zever/orm v0.0.0
	github.com/zenta-dev/zever/shared/registry v0.0.0
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.11.0 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	github.com/zenta-dev/zever/adapters/log/noop v0.0.0 // indirect
	github.com/zenta-dev/zever/core/ai v0.0.0 // indirect
	github.com/zenta-dev/zever/core/analytics v0.0.0 // indirect
	github.com/zenta-dev/zever/core/auth v0.0.0 // indirect
	github.com/zenta-dev/zever/core/billing v0.0.0 // indirect
	github.com/zenta-dev/zever/core/cache v0.0.0 // indirect
	github.com/zenta-dev/zever/core/crypto v0.0.0 // indirect
	github.com/zenta-dev/zever/core/document v0.0.0 // indirect
	github.com/zenta-dev/zever/core/eventbus v0.0.0 // indirect
	github.com/zenta-dev/zever/core/flag v0.0.0 // indirect
	github.com/zenta-dev/zever/core/geo v0.0.0 // indirect
	github.com/zenta-dev/zever/core/i18n v0.0.0 // indirect
	github.com/zenta-dev/zever/core/idempotency v0.0.0 // indirect
	github.com/zenta-dev/zever/core/job v0.0.0 // indirect
	github.com/zenta-dev/zever/core/lock v0.0.0 // indirect
	github.com/zenta-dev/zever/core/log v0.0.0 // indirect
	github.com/zenta-dev/zever/core/mailer v0.0.0 // indirect
	github.com/zenta-dev/zever/core/media v0.0.0 // indirect
	github.com/zenta-dev/zever/core/notification v0.0.0 // indirect
	github.com/zenta-dev/zever/core/observability v0.0.0 // indirect
	github.com/zenta-dev/zever/core/password v0.0.0 // indirect
	github.com/zenta-dev/zever/core/payment v0.0.0 // indirect
	github.com/zenta-dev/zever/core/permission v0.0.0 // indirect
	github.com/zenta-dev/zever/core/queue v0.0.0 // indirect
	github.com/zenta-dev/zever/core/ratelimit v0.0.0 // indirect
	github.com/zenta-dev/zever/core/router v0.0.0 // indirect
	github.com/zenta-dev/zever/core/scheduler v0.0.0 // indirect
	github.com/zenta-dev/zever/core/search v0.0.0 // indirect
	github.com/zenta-dev/zever/core/secrets v0.0.0 // indirect
	github.com/zenta-dev/zever/core/session v0.0.0 // indirect
	github.com/zenta-dev/zever/core/storage v0.0.0 // indirect
	github.com/zenta-dev/zever/core/tenant v0.0.0 // indirect
	github.com/zenta-dev/zever/core/vectorstore v0.0.0 // indirect
	github.com/zenta-dev/zever/core/webhook v0.0.0 // indirect
	github.com/zenta-dev/zever/core/workflow v0.0.0 // indirect
	github.com/zenta-dev/zever/shared/codec v0.0.0 // indirect
	github.com/zenta-dev/zever/shared/endpoint v0.0.0 // indirect
	github.com/zenta-dev/zever/shared/httpclient v0.0.0 // indirect
	github.com/zenta-dev/zever/shared/lrucache v0.0.0 // indirect
	github.com/zenta-dev/zever/shared/providersopt v0.0.0 // indirect
	github.com/zenta-dev/zever/shared/redisopt v0.0.0 // indirect
	github.com/zenta-dev/zever/shared/retry v0.0.0 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260526163538-3dc84a4a5aaa // indirect
	google.golang.org/grpc v1.83.2 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
	modernc.org/libc v1.75.7 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
	modernc.org/sqlite v1.59.0 // indirect
)

replace (
	github.com/zenta-dev/zever/adapters/db/sqlite => ../adapters/db/sqlite
	github.com/zenta-dev/zever/config => ../config
	github.com/zenta-dev/zever/container => ../container
	github.com/zenta-dev/zever/core/db => ../core/db
	github.com/zenta-dev/zever/orm => ../orm
	github.com/zenta-dev/zever/shared/registry => ../shared/registry
)

replace github.com/zenta-dev/zever/adapters/analytics/log => ../adapters/analytics/log

replace github.com/zenta-dev/zever/adapters/auth/session => ../adapters/auth/session

replace github.com/zenta-dev/zever/adapters/cache/memory => ../adapters/cache/memory

replace github.com/zenta-dev/zever/adapters/crypto/local => ../adapters/crypto/local

replace github.com/zenta-dev/zever/adapters/db/postgres => ../adapters/db/postgres

replace github.com/zenta-dev/zever/adapters/eventbus/memory => ../adapters/eventbus/memory

replace github.com/zenta-dev/zever/adapters/flag/static => ../adapters/flag/static

replace github.com/zenta-dev/zever/adapters/geo/static => ../adapters/geo/static

replace github.com/zenta-dev/zever/adapters/i18n/embed => ../adapters/i18n/embed

replace github.com/zenta-dev/zever/adapters/idempotency/memory => ../adapters/idempotency/memory

replace github.com/zenta-dev/zever/adapters/lock/memory => ../adapters/lock/memory

replace github.com/zenta-dev/zever/adapters/log/noop => ../adapters/log/noop

replace github.com/zenta-dev/zever/adapters/mailer/log => ../adapters/mailer/log

replace github.com/zenta-dev/zever/adapters/notification/log => ../adapters/notification/log

replace github.com/zenta-dev/zever/adapters/observability/noop => ../adapters/observability/noop

replace github.com/zenta-dev/zever/adapters/payment/stub => ../adapters/payment/stub

replace github.com/zenta-dev/zever/adapters/permission/noop => ../adapters/permission/noop

replace github.com/zenta-dev/zever/adapters/queue/memory => ../adapters/queue/memory

replace github.com/zenta-dev/zever/adapters/ratelimit/memory => ../adapters/ratelimit/memory

replace github.com/zenta-dev/zever/adapters/router/stdhttp => ../adapters/router/stdhttp

replace github.com/zenta-dev/zever/adapters/secrets/env => ../adapters/secrets/env

replace github.com/zenta-dev/zever/adapters/session/memory => ../adapters/session/memory

replace github.com/zenta-dev/zever/adapters/storage/local => ../adapters/storage/local

replace github.com/zenta-dev/zever/adapters/tenant/single => ../adapters/tenant/single

replace github.com/zenta-dev/zever/adapters/webhook/http => ../adapters/webhook/http

replace github.com/zenta-dev/zever/adapters/workflow/memory => ../adapters/workflow/memory

replace github.com/zenta-dev/zever/core/ai => ../core/ai

replace github.com/zenta-dev/zever/core/analytics => ../core/analytics

replace github.com/zenta-dev/zever/core/auth => ../core/auth

replace github.com/zenta-dev/zever/core/authz => ../core/authz

replace github.com/zenta-dev/zever/core/billing => ../core/billing

replace github.com/zenta-dev/zever/core/cache => ../core/cache

replace github.com/zenta-dev/zever/core/crypto => ../core/crypto

replace github.com/zenta-dev/zever/core/document => ../core/document

replace github.com/zenta-dev/zever/core/eventbus => ../core/eventbus

replace github.com/zenta-dev/zever/core/flag => ../core/flag

replace github.com/zenta-dev/zever/core/geo => ../core/geo

replace github.com/zenta-dev/zever/core/i18n => ../core/i18n

replace github.com/zenta-dev/zever/core/idempotency => ../core/idempotency

replace github.com/zenta-dev/zever/core/job => ../core/job

replace github.com/zenta-dev/zever/core/lock => ../core/lock

replace github.com/zenta-dev/zever/core/log => ../core/log

replace github.com/zenta-dev/zever/core/mailer => ../core/mailer

replace github.com/zenta-dev/zever/core/media => ../core/media

replace github.com/zenta-dev/zever/core/notification => ../core/notification

replace github.com/zenta-dev/zever/core/observability => ../core/observability

replace github.com/zenta-dev/zever/core/password => ../core/password

replace github.com/zenta-dev/zever/core/payment => ../core/payment

replace github.com/zenta-dev/zever/core/permission => ../core/permission

replace github.com/zenta-dev/zever/core/queue => ../core/queue

replace github.com/zenta-dev/zever/core/ratelimit => ../core/ratelimit

replace github.com/zenta-dev/zever/core/router => ../core/router

replace github.com/zenta-dev/zever/core/scheduler => ../core/scheduler

replace github.com/zenta-dev/zever/core/search => ../core/search

replace github.com/zenta-dev/zever/core/secrets => ../core/secrets

replace github.com/zenta-dev/zever/core/session => ../core/session

replace github.com/zenta-dev/zever/core/storage => ../core/storage

replace github.com/zenta-dev/zever/core/tenant => ../core/tenant

replace github.com/zenta-dev/zever/core/vectorstore => ../core/vectorstore

replace github.com/zenta-dev/zever/core/webhook => ../core/webhook

replace github.com/zenta-dev/zever/core/workflow => ../core/workflow

replace github.com/zenta-dev/zever/dsl => ../dsl

replace github.com/zenta-dev/zever/shared/apperror => ../shared/apperror

replace github.com/zenta-dev/zever/shared/codec => ../shared/codec

replace github.com/zenta-dev/zever/shared/endpoint => ../shared/endpoint

replace github.com/zenta-dev/zever/shared/httpclient => ../shared/httpclient

replace github.com/zenta-dev/zever/shared/lrucache => ../shared/lrucache

replace github.com/zenta-dev/zever/shared/providersopt => ../shared/providersopt

replace github.com/zenta-dev/zever/shared/redisclient => ../shared/redisclient

replace github.com/zenta-dev/zever/shared/redisopt => ../shared/redisopt

replace github.com/zenta-dev/zever/shared/retry => ../shared/retry
