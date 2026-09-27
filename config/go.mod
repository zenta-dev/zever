module github.com/zenta-dev/zever/config

go 1.27.0

require (
	github.com/zenta-dev/zever/core/ai v0.0.0
	github.com/zenta-dev/zever/core/analytics v0.0.0
	github.com/zenta-dev/zever/core/auth v0.0.0
	github.com/zenta-dev/zever/core/billing v0.0.0
	github.com/zenta-dev/zever/core/cache v0.0.0
	github.com/zenta-dev/zever/core/crypto v0.0.0
	github.com/zenta-dev/zever/core/db v0.0.0
	github.com/zenta-dev/zever/core/document v0.0.0
	github.com/zenta-dev/zever/core/eventbus v0.0.0
	github.com/zenta-dev/zever/core/flag v0.0.0
	github.com/zenta-dev/zever/core/geo v0.0.0
	github.com/zenta-dev/zever/core/i18n v0.0.0
	github.com/zenta-dev/zever/core/idempotency v0.0.0
	github.com/zenta-dev/zever/core/job v0.0.0
	github.com/zenta-dev/zever/core/lock v0.0.0
	github.com/zenta-dev/zever/core/log v0.0.0
	github.com/zenta-dev/zever/core/mailer v0.0.0
	github.com/zenta-dev/zever/core/media v0.0.0
	github.com/zenta-dev/zever/core/notification v0.0.0
	github.com/zenta-dev/zever/core/observability v0.0.0
	github.com/zenta-dev/zever/core/password v0.0.0
	github.com/zenta-dev/zever/core/payment v0.0.0
	github.com/zenta-dev/zever/core/permission v0.0.0
	github.com/zenta-dev/zever/core/queue v0.0.0
	github.com/zenta-dev/zever/core/ratelimit v0.0.0
	github.com/zenta-dev/zever/core/router v0.0.0
	github.com/zenta-dev/zever/core/scheduler v0.0.0
	github.com/zenta-dev/zever/core/search v0.0.0
	github.com/zenta-dev/zever/core/secrets v0.0.0
	github.com/zenta-dev/zever/core/session v0.0.0
	github.com/zenta-dev/zever/core/storage v0.0.0
	github.com/zenta-dev/zever/core/tenant v0.0.0
	github.com/zenta-dev/zever/core/vectorstore v0.0.0
	github.com/zenta-dev/zever/core/webhook v0.0.0
	github.com/zenta-dev/zever/core/workflow v0.0.0
	github.com/zenta-dev/zever/shared/codec v0.0.0
	github.com/zenta-dev/zever/shared/redisopt v0.0.0
	go.yaml.in/yaml/v3 v3.0.5
)

require (
	github.com/robfig/cron/v3 v3.0.1 // indirect
	github.com/zenta-dev/zever/adapters/log/noop v0.0.0 // indirect
	github.com/zenta-dev/zever/shared/endpoint v0.0.0 // indirect
	github.com/zenta-dev/zever/shared/httpclient v0.0.0 // indirect
	github.com/zenta-dev/zever/shared/providersopt v0.0.0 // indirect
	github.com/zenta-dev/zever/shared/registry v0.0.0 // indirect
	github.com/zenta-dev/zever/shared/retry v0.0.0 // indirect
)

replace (
	github.com/zenta-dev/zever/adapters/log/noop => ../adapters/log/noop
	github.com/zenta-dev/zever/core/ai => ../core/ai
	github.com/zenta-dev/zever/core/analytics => ../core/analytics
	github.com/zenta-dev/zever/core/auth => ../core/auth
	github.com/zenta-dev/zever/core/billing => ../core/billing
	github.com/zenta-dev/zever/core/cache => ../core/cache
	github.com/zenta-dev/zever/core/crypto => ../core/crypto
	github.com/zenta-dev/zever/core/db => ../core/db
	github.com/zenta-dev/zever/core/document => ../core/document
	github.com/zenta-dev/zever/core/eventbus => ../core/eventbus
	github.com/zenta-dev/zever/core/flag => ../core/flag
	github.com/zenta-dev/zever/core/geo => ../core/geo
	github.com/zenta-dev/zever/core/i18n => ../core/i18n
	github.com/zenta-dev/zever/core/idempotency => ../core/idempotency
	github.com/zenta-dev/zever/core/job => ../core/job
	github.com/zenta-dev/zever/core/lock => ../core/lock
	github.com/zenta-dev/zever/core/log => ../core/log
	github.com/zenta-dev/zever/core/mailer => ../core/mailer
	github.com/zenta-dev/zever/core/media => ../core/media
	github.com/zenta-dev/zever/core/notification => ../core/notification
	github.com/zenta-dev/zever/core/observability => ../core/observability
	github.com/zenta-dev/zever/core/password => ../core/password
	github.com/zenta-dev/zever/core/payment => ../core/payment
	github.com/zenta-dev/zever/core/permission => ../core/permission
	github.com/zenta-dev/zever/core/queue => ../core/queue
	github.com/zenta-dev/zever/core/ratelimit => ../core/ratelimit
	github.com/zenta-dev/zever/core/router => ../core/router
	github.com/zenta-dev/zever/core/scheduler => ../core/scheduler
	github.com/zenta-dev/zever/core/search => ../core/search
	github.com/zenta-dev/zever/core/secrets => ../core/secrets
	github.com/zenta-dev/zever/core/session => ../core/session
	github.com/zenta-dev/zever/core/storage => ../core/storage
	github.com/zenta-dev/zever/core/tenant => ../core/tenant
	github.com/zenta-dev/zever/core/vectorstore => ../core/vectorstore
	github.com/zenta-dev/zever/core/webhook => ../core/webhook
	github.com/zenta-dev/zever/core/workflow => ../core/workflow
	github.com/zenta-dev/zever/shared/codec => ../shared/codec
	github.com/zenta-dev/zever/shared/endpoint => ../shared/endpoint
	github.com/zenta-dev/zever/shared/httpclient => ../shared/httpclient
	github.com/zenta-dev/zever/shared/providersopt => ../shared/providersopt
	github.com/zenta-dev/zever/shared/redisopt => ../shared/redisopt
	github.com/zenta-dev/zever/shared/registry => ../shared/registry
	github.com/zenta-dev/zever/shared/retry => ../shared/retry
)

replace github.com/zenta-dev/zever/adapters/analytics/log => ../adapters/analytics/log

replace github.com/zenta-dev/zever/adapters/auth/session => ../adapters/auth/session

replace github.com/zenta-dev/zever/adapters/cache/memory => ../adapters/cache/memory

replace github.com/zenta-dev/zever/adapters/crypto/local => ../adapters/crypto/local

replace github.com/zenta-dev/zever/adapters/eventbus/memory => ../adapters/eventbus/memory

replace github.com/zenta-dev/zever/adapters/flag/static => ../adapters/flag/static

replace github.com/zenta-dev/zever/adapters/geo/static => ../adapters/geo/static

replace github.com/zenta-dev/zever/adapters/i18n/embed => ../adapters/i18n/embed

replace github.com/zenta-dev/zever/adapters/idempotency/memory => ../adapters/idempotency/memory

replace github.com/zenta-dev/zever/adapters/lock/memory => ../adapters/lock/memory

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

replace github.com/zenta-dev/zever/shared/redisclient => ../shared/redisclient
