module github.com/zenta-dev/zever/examples/showcase

go 1.27.0

require (
	github.com/google/uuid v1.6.0
	github.com/zenta-dev/zever/adapters/ai/anthropic v0.0.0
	github.com/zenta-dev/zever/adapters/analytics/log v0.0.0
	github.com/zenta-dev/zever/adapters/auth/jwt v0.0.0
	github.com/zenta-dev/zever/adapters/billing/stub v0.0.0
	github.com/zenta-dev/zever/adapters/cache/memory v0.0.0
	github.com/zenta-dev/zever/adapters/crypto/local v0.0.0
	github.com/zenta-dev/zever/adapters/db/sqlite v0.0.0
	github.com/zenta-dev/zever/adapters/document/local v0.0.0
	github.com/zenta-dev/zever/adapters/eventbus/memory v0.0.0
	github.com/zenta-dev/zever/adapters/flag/static v0.0.0
	github.com/zenta-dev/zever/adapters/geo/static v0.0.0
	github.com/zenta-dev/zever/adapters/i18n/embed v0.0.0
	github.com/zenta-dev/zever/adapters/idempotency/memory v0.0.0
	github.com/zenta-dev/zever/adapters/lock/memory v0.0.0
	github.com/zenta-dev/zever/adapters/log/slog v0.0.0
	github.com/zenta-dev/zever/adapters/mailer/log v0.0.0
	github.com/zenta-dev/zever/adapters/media/local v0.0.0
	github.com/zenta-dev/zever/adapters/notification/log v0.0.0
	github.com/zenta-dev/zever/adapters/observability/stdout v0.0.0
	github.com/zenta-dev/zever/adapters/password/argon2 v0.0.0
	github.com/zenta-dev/zever/adapters/payment/stub v0.0.0
	github.com/zenta-dev/zever/adapters/permission/rbac v0.0.0
	github.com/zenta-dev/zever/adapters/queue/memory v0.0.0
	github.com/zenta-dev/zever/adapters/ratelimit/memory v0.0.0
	github.com/zenta-dev/zever/adapters/router/stdhttp v0.0.0
	github.com/zenta-dev/zever/adapters/scheduler/embedded v0.0.0
	github.com/zenta-dev/zever/adapters/search/sqlite v0.0.0
	github.com/zenta-dev/zever/adapters/secrets/env v0.0.0
	github.com/zenta-dev/zever/adapters/session/memory v0.0.0
	github.com/zenta-dev/zever/adapters/storage/local v0.0.0
	github.com/zenta-dev/zever/adapters/tenant/single v0.0.0
	github.com/zenta-dev/zever/adapters/vectorstore/sqlite v0.0.0
	github.com/zenta-dev/zever/adapters/webhook/http v0.0.0
	github.com/zenta-dev/zever/adapters/workflow/memory v0.0.0
	github.com/zenta-dev/zever/config v0.0.0
	github.com/zenta-dev/zever/container v0.0.0
	github.com/zenta-dev/zever/core/auth v0.0.0
	github.com/zenta-dev/zever/core/authz v0.0.0
	github.com/zenta-dev/zever/core/db v0.0.0
	github.com/zenta-dev/zever/core/flag v0.0.0
	github.com/zenta-dev/zever/core/i18n v0.0.0
	github.com/zenta-dev/zever/core/job v0.0.0
	github.com/zenta-dev/zever/core/log v0.0.0
	github.com/zenta-dev/zever/core/mailer v0.0.0
	github.com/zenta-dev/zever/core/middleware v0.0.0
	github.com/zenta-dev/zever/core/notification v0.0.0
	github.com/zenta-dev/zever/core/password v0.0.0
	github.com/zenta-dev/zever/core/permission v0.0.0
	github.com/zenta-dev/zever/core/queue v0.0.0
	github.com/zenta-dev/zever/core/ratelimit v0.0.0
	github.com/zenta-dev/zever/core/router v0.0.0
	github.com/zenta-dev/zever/orm v0.0.0
	github.com/zenta-dev/zever/shared/apperror v0.0.0
	google.golang.org/genproto/googleapis/api v0.0.0-20260819154853-08b0e4226688
	google.golang.org/grpc v1.83.2
	google.golang.org/protobuf v1.36.12
)

require (
	github.com/HugoSmits86/nativewebp v1.3.0 // indirect
	github.com/anthonynsimon/bild v0.17.1 // indirect
	github.com/anthropics/anthropic-sdk-go v1.73.0 // indirect
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/buger/jsonparser v1.1.2 // indirect
	github.com/chromedp/cdproto v0.0.0-20260714215040-dc233986426f // indirect
	github.com/chromedp/chromedp v0.16.0 // indirect
	github.com/chromedp/sysutil v1.1.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/go-json-experiment/json v0.0.0-20260623181947-01eb4420fa68 // indirect
	github.com/gobwas/httphead v0.1.0 // indirect
	github.com/gobwas/pool v0.2.1 // indirect
	github.com/gobwas/ws v1.4.0 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/invopop/jsonschema v0.14.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.11.0 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/oklog/ulid/v2 v2.1.2 // indirect
	github.com/pb33f/ordered-map/v2 v2.3.1 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	github.com/standard-webhooks/standard-webhooks/libraries v0.0.1 // indirect
	github.com/tidwall/gjson v1.19.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	github.com/zenta-dev/zever/adapters/log/noop v0.0.0 // indirect
	github.com/zenta-dev/zever/adapters/media/ffmpeg v0.0.0 // indirect
	github.com/zenta-dev/zever/core/ai v0.0.0 // indirect
	github.com/zenta-dev/zever/core/analytics v0.0.0 // indirect
	github.com/zenta-dev/zever/core/billing v0.0.0 // indirect
	github.com/zenta-dev/zever/core/cache v0.0.0 // indirect
	github.com/zenta-dev/zever/core/crypto v0.0.0 // indirect
	github.com/zenta-dev/zever/core/document v0.0.0 // indirect
	github.com/zenta-dev/zever/core/eventbus v0.0.0 // indirect
	github.com/zenta-dev/zever/core/geo v0.0.0 // indirect
	github.com/zenta-dev/zever/core/idempotency v0.0.0 // indirect
	github.com/zenta-dev/zever/core/lock v0.0.0 // indirect
	github.com/zenta-dev/zever/core/media v0.0.0 // indirect
	github.com/zenta-dev/zever/core/observability v0.0.0 // indirect
	github.com/zenta-dev/zever/core/payment v0.0.0 // indirect
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
	github.com/zenta-dev/zever/shared/registry v0.0.0 // indirect
	github.com/zenta-dev/zever/shared/retry v0.0.0 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	go.yaml.in/yaml/v4 v4.0.0-rc.2 // indirect
	golang.org/x/crypto v0.57.0 // indirect
	golang.org/x/image v0.45.0 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260818201246-1b0934165a6f // indirect
	modernc.org/libc v1.75.7 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
	modernc.org/sqlite v1.59.0 // indirect
)

replace (
	github.com/zenta-dev/zever/adapters/auth/jwt => ../../adapters/auth/jwt
	github.com/zenta-dev/zever/adapters/billing/stub => ../../adapters/billing/stub
	github.com/zenta-dev/zever/adapters/cache/memory => ../../adapters/cache/memory
	github.com/zenta-dev/zever/adapters/db/sqlite => ../../adapters/db/sqlite
	github.com/zenta-dev/zever/adapters/document/local => ../../adapters/document/local
	github.com/zenta-dev/zever/adapters/log/slog => ../../adapters/log/slog
	github.com/zenta-dev/zever/adapters/media/local => ../../adapters/media/local
	github.com/zenta-dev/zever/adapters/password/argon2 => ../../adapters/password/argon2
	github.com/zenta-dev/zever/adapters/permission/rbac => ../../adapters/permission/rbac
	github.com/zenta-dev/zever/adapters/queue/memory => ../../adapters/queue/memory
	github.com/zenta-dev/zever/adapters/scheduler/embedded => ../../adapters/scheduler/embedded
	github.com/zenta-dev/zever/adapters/search/sqlite => ../../adapters/search/sqlite
	github.com/zenta-dev/zever/adapters/vectorstore/sqlite => ../../adapters/vectorstore/sqlite
	github.com/zenta-dev/zever/config => ../../config
	github.com/zenta-dev/zever/container => ../../container
	github.com/zenta-dev/zever/core/auth => ../../core/auth
	github.com/zenta-dev/zever/core/authz => ../../core/authz
	github.com/zenta-dev/zever/core/db => ../../core/db
	github.com/zenta-dev/zever/core/flag => ../../core/flag
	github.com/zenta-dev/zever/core/i18n => ../../core/i18n
	github.com/zenta-dev/zever/core/job => ../../core/job
	github.com/zenta-dev/zever/core/log => ../../core/log
	github.com/zenta-dev/zever/core/mailer => ../../core/mailer
	github.com/zenta-dev/zever/core/middleware => ../../core/middleware
	github.com/zenta-dev/zever/core/notification => ../../core/notification
	github.com/zenta-dev/zever/core/password => ../../core/password
	github.com/zenta-dev/zever/core/permission => ../../core/permission
	github.com/zenta-dev/zever/core/queue => ../../core/queue
	github.com/zenta-dev/zever/core/ratelimit => ../../core/ratelimit
	github.com/zenta-dev/zever/core/router => ../../core/router
	github.com/zenta-dev/zever/orm => ../../orm
	github.com/zenta-dev/zever/shared/apperror => ../../shared/apperror
)

replace github.com/zenta-dev/zever/adapters/analytics/log => ../../adapters/analytics/log

replace github.com/zenta-dev/zever/adapters/auth/session => ../../adapters/auth/session

replace github.com/zenta-dev/zever/adapters/crypto/local => ../../adapters/crypto/local

replace github.com/zenta-dev/zever/adapters/db/postgres => ../../adapters/db/postgres

replace github.com/zenta-dev/zever/adapters/eventbus/memory => ../../adapters/eventbus/memory

replace github.com/zenta-dev/zever/adapters/flag/static => ../../adapters/flag/static

replace github.com/zenta-dev/zever/adapters/geo/static => ../../adapters/geo/static

replace github.com/zenta-dev/zever/adapters/i18n/embed => ../../adapters/i18n/embed

replace github.com/zenta-dev/zever/adapters/idempotency/memory => ../../adapters/idempotency/memory

replace github.com/zenta-dev/zever/adapters/lock/memory => ../../adapters/lock/memory

replace github.com/zenta-dev/zever/adapters/log/noop => ../../adapters/log/noop

replace github.com/zenta-dev/zever/adapters/mailer/log => ../../adapters/mailer/log

replace github.com/zenta-dev/zever/adapters/media/ffmpeg => ../../adapters/media/ffmpeg

replace github.com/zenta-dev/zever/adapters/notification/log => ../../adapters/notification/log

replace github.com/zenta-dev/zever/adapters/observability/noop => ../../adapters/observability/noop

replace github.com/zenta-dev/zever/adapters/payment/stub => ../../adapters/payment/stub

replace github.com/zenta-dev/zever/adapters/permission/noop => ../../adapters/permission/noop

replace github.com/zenta-dev/zever/adapters/ratelimit/memory => ../../adapters/ratelimit/memory

replace github.com/zenta-dev/zever/adapters/router/stdhttp => ../../adapters/router/stdhttp

replace github.com/zenta-dev/zever/adapters/secrets/env => ../../adapters/secrets/env

replace github.com/zenta-dev/zever/adapters/session/memory => ../../adapters/session/memory

replace github.com/zenta-dev/zever/adapters/storage/local => ../../adapters/storage/local

replace github.com/zenta-dev/zever/adapters/tenant/single => ../../adapters/tenant/single

replace github.com/zenta-dev/zever/adapters/webhook/http => ../../adapters/webhook/http

replace github.com/zenta-dev/zever/adapters/workflow/memory => ../../adapters/workflow/memory

replace github.com/zenta-dev/zever/core/ai => ../../core/ai

replace github.com/zenta-dev/zever/core/analytics => ../../core/analytics

replace github.com/zenta-dev/zever/core/billing => ../../core/billing

replace github.com/zenta-dev/zever/core/cache => ../../core/cache

replace github.com/zenta-dev/zever/core/crypto => ../../core/crypto

replace github.com/zenta-dev/zever/core/document => ../../core/document

replace github.com/zenta-dev/zever/core/eventbus => ../../core/eventbus

replace github.com/zenta-dev/zever/core/geo => ../../core/geo

replace github.com/zenta-dev/zever/core/idempotency => ../../core/idempotency

replace github.com/zenta-dev/zever/core/lock => ../../core/lock

replace github.com/zenta-dev/zever/core/media => ../../core/media

replace github.com/zenta-dev/zever/core/observability => ../../core/observability

replace github.com/zenta-dev/zever/core/payment => ../../core/payment

replace github.com/zenta-dev/zever/core/scheduler => ../../core/scheduler

replace github.com/zenta-dev/zever/core/search => ../../core/search

replace github.com/zenta-dev/zever/core/secrets => ../../core/secrets

replace github.com/zenta-dev/zever/core/session => ../../core/session

replace github.com/zenta-dev/zever/core/storage => ../../core/storage

replace github.com/zenta-dev/zever/core/tenant => ../../core/tenant

replace github.com/zenta-dev/zever/core/vectorstore => ../../core/vectorstore

replace github.com/zenta-dev/zever/core/webhook => ../../core/webhook

replace github.com/zenta-dev/zever/core/workflow => ../../core/workflow

replace github.com/zenta-dev/zever/dsl => ../../dsl

replace github.com/zenta-dev/zever/shared/codec => ../../shared/codec

replace github.com/zenta-dev/zever/shared/endpoint => ../../shared/endpoint

replace github.com/zenta-dev/zever/shared/httpclient => ../../shared/httpclient

replace github.com/zenta-dev/zever/shared/lrucache => ../../shared/lrucache

replace github.com/zenta-dev/zever/shared/providersopt => ../../shared/providersopt

replace github.com/zenta-dev/zever/shared/redisclient => ../../shared/redisclient

replace github.com/zenta-dev/zever/shared/redisopt => ../../shared/redisopt

replace github.com/zenta-dev/zever/shared/registry => ../../shared/registry

replace github.com/zenta-dev/zever/shared/retry => ../../shared/retry

replace github.com/zenta-dev/zever/adapters/ai/anthropic => ../../adapters/ai/anthropic

replace github.com/zenta-dev/zever/adapters/observability/stdout => ../../adapters/observability/stdout
