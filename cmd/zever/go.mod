module github.com/zenta-dev/zever/cmd/zever

go 1.27.0

require (
	charm.land/bubbletea/v2 v2.0.10
	charm.land/huh/v2 v2.0.3
	github.com/charmbracelet/x/exp/teatest/v2 v2.0.0-20260924144451-d676b019604b
	github.com/fsnotify/fsnotify v1.10.1
	github.com/spf13/cobra v1.10.2
	github.com/spf13/pflag v1.0.9
	github.com/traefik/yaegi v0.16.1
	github.com/zenta-dev/zever/adapters/ai/anthropic v0.5.3
	github.com/zenta-dev/zever/adapters/ai/gemini v0.5.3
	github.com/zenta-dev/zever/adapters/ai/ollama v0.5.3
	github.com/zenta-dev/zever/adapters/ai/openai v0.5.3
	github.com/zenta-dev/zever/adapters/analytics/log v0.5.3
	github.com/zenta-dev/zever/adapters/analytics/posthog v0.5.3
	github.com/zenta-dev/zever/adapters/auth/jwt v0.5.3
	github.com/zenta-dev/zever/adapters/auth/oidc v0.5.3
	github.com/zenta-dev/zever/adapters/auth/session v0.5.3
	github.com/zenta-dev/zever/adapters/billing/paddle v0.5.3
	github.com/zenta-dev/zever/adapters/billing/stripe v0.5.3
	github.com/zenta-dev/zever/adapters/billing/stub v0.5.3
	github.com/zenta-dev/zever/adapters/cache/db v0.5.3
	github.com/zenta-dev/zever/adapters/cache/memory v0.5.3
	github.com/zenta-dev/zever/adapters/cache/redis v0.5.3
	github.com/zenta-dev/zever/adapters/cdn/cloudflare v0.5.3
	github.com/zenta-dev/zever/adapters/cdn/noop v0.5.3
	github.com/zenta-dev/zever/adapters/crypto/kms v0.5.3
	github.com/zenta-dev/zever/adapters/crypto/local v0.5.3
	github.com/zenta-dev/zever/adapters/db/postgres v0.5.3
	github.com/zenta-dev/zever/adapters/db/sqlite v0.5.3
	github.com/zenta-dev/zever/adapters/document/latex v0.5.3
	github.com/zenta-dev/zever/adapters/document/local v0.5.3
	github.com/zenta-dev/zever/adapters/document/remote v0.5.3
	github.com/zenta-dev/zever/adapters/eventbus/memory v0.5.3
	github.com/zenta-dev/zever/adapters/eventbus/redis v0.5.3
	github.com/zenta-dev/zever/adapters/flag/firebase v0.5.3
	github.com/zenta-dev/zever/adapters/flag/static v0.5.3
	github.com/zenta-dev/zever/adapters/geo/google v0.5.3
	github.com/zenta-dev/zever/adapters/geo/osm v0.5.3
	github.com/zenta-dev/zever/adapters/geo/static v0.5.3
	github.com/zenta-dev/zever/adapters/i18n/embed v0.5.3
	github.com/zenta-dev/zever/adapters/i18n/remote v0.5.3
	github.com/zenta-dev/zever/adapters/idempotency/db v0.5.3
	github.com/zenta-dev/zever/adapters/idempotency/memory v0.5.3
	github.com/zenta-dev/zever/adapters/idempotency/redis v0.5.3
	github.com/zenta-dev/zever/adapters/lock/memory v0.5.3
	github.com/zenta-dev/zever/adapters/lock/redis v0.5.3
	github.com/zenta-dev/zever/adapters/log/noop v0.5.3
	github.com/zenta-dev/zever/adapters/log/pretty v0.5.3
	github.com/zenta-dev/zever/adapters/log/slog v0.5.3
	github.com/zenta-dev/zever/adapters/log/zerolog v0.5.3
	github.com/zenta-dev/zever/adapters/mailer/log v0.5.3
	github.com/zenta-dev/zever/adapters/mailer/smtp v0.5.3
	github.com/zenta-dev/zever/adapters/media/local v0.5.3
	github.com/zenta-dev/zever/adapters/media/s3 v0.5.3
	github.com/zenta-dev/zever/adapters/notification/fcm v0.5.3
	github.com/zenta-dev/zever/adapters/notification/log v0.5.3
	github.com/zenta-dev/zever/adapters/notification/twilio v0.5.3
	github.com/zenta-dev/zever/adapters/observability/noop v0.5.3
	github.com/zenta-dev/zever/adapters/observability/otlp v0.5.3
	github.com/zenta-dev/zever/adapters/observability/stdout v0.5.3
	github.com/zenta-dev/zever/adapters/outbox/cdc v0.5.3
	github.com/zenta-dev/zever/adapters/outbox/db v0.5.3
	github.com/zenta-dev/zever/adapters/outbox/memory v0.5.3
	github.com/zenta-dev/zever/adapters/password/argon2 v0.5.3
	github.com/zenta-dev/zever/adapters/payment/paddle v0.5.3
	github.com/zenta-dev/zever/adapters/payment/stripe v0.5.3
	github.com/zenta-dev/zever/adapters/payment/stub v0.5.3
	github.com/zenta-dev/zever/adapters/permission/casbin v0.5.3
	github.com/zenta-dev/zever/adapters/permission/noop v0.5.3
	github.com/zenta-dev/zever/adapters/permission/rbac v0.5.3
	github.com/zenta-dev/zever/adapters/queue/db v0.5.3
	github.com/zenta-dev/zever/adapters/queue/memory v0.5.3
	github.com/zenta-dev/zever/adapters/queue/redis v0.5.3
	github.com/zenta-dev/zever/adapters/ratelimit/memory v0.5.3
	github.com/zenta-dev/zever/adapters/ratelimit/redis v0.5.3
	github.com/zenta-dev/zever/adapters/resilience/inproc v0.5.3
	github.com/zenta-dev/zever/adapters/resilience/redis v0.5.3
	github.com/zenta-dev/zever/adapters/router/fiber v0.5.3
	github.com/zenta-dev/zever/adapters/router/stdhttp v0.5.3
	github.com/zenta-dev/zever/adapters/scheduler/embedded v0.5.3
	github.com/zenta-dev/zever/adapters/scheduler/postgres v0.5.3
	github.com/zenta-dev/zever/adapters/search/db v0.5.3
	github.com/zenta-dev/zever/adapters/search/meilisearch v0.5.3
	github.com/zenta-dev/zever/adapters/secrets/env v0.5.3
	github.com/zenta-dev/zever/adapters/secrets/vault v0.5.3
	github.com/zenta-dev/zever/adapters/session/db v0.5.3
	github.com/zenta-dev/zever/adapters/session/memory v0.5.3
	github.com/zenta-dev/zever/adapters/session/redis v0.5.3
	github.com/zenta-dev/zever/adapters/storage/local v0.5.3
	github.com/zenta-dev/zever/adapters/storage/r2 v0.5.3
	github.com/zenta-dev/zever/adapters/storage/s3 v0.5.3
	github.com/zenta-dev/zever/adapters/tenant/header v0.5.3
	github.com/zenta-dev/zever/adapters/tenant/single v0.5.3
	github.com/zenta-dev/zever/adapters/vectorstore/db v0.5.3
	github.com/zenta-dev/zever/adapters/vectorstore/qdrant v0.5.3
	github.com/zenta-dev/zever/adapters/webhook/http v0.5.3
	github.com/zenta-dev/zever/adapters/webhook/queue v0.5.3
	github.com/zenta-dev/zever/adapters/workflow/db v0.5.3
	github.com/zenta-dev/zever/adapters/workflow/memory v0.5.3
	github.com/zenta-dev/zever/config v0.5.3
	github.com/zenta-dev/zever/container v0.5.3
	github.com/zenta-dev/zever/core/ai v0.5.3
	github.com/zenta-dev/zever/core/analytics v0.5.3
	github.com/zenta-dev/zever/core/auth v0.5.3
	github.com/zenta-dev/zever/core/cache v0.5.3
	github.com/zenta-dev/zever/core/cdn v0.5.3
	github.com/zenta-dev/zever/core/crypto v0.5.3
	github.com/zenta-dev/zever/core/db v0.5.3
	github.com/zenta-dev/zever/core/document v0.5.3
	github.com/zenta-dev/zever/core/eventbus v0.5.3
	github.com/zenta-dev/zever/core/flag v0.5.3
	github.com/zenta-dev/zever/core/geo v0.5.3
	github.com/zenta-dev/zever/core/i18n v0.5.3
	github.com/zenta-dev/zever/core/idempotency v0.5.3
	github.com/zenta-dev/zever/core/lock v0.5.3
	github.com/zenta-dev/zever/core/log v0.5.3
	github.com/zenta-dev/zever/core/mailer v0.5.3
	github.com/zenta-dev/zever/core/notification v0.5.3
	github.com/zenta-dev/zever/core/observability v0.5.3
	github.com/zenta-dev/zever/core/outbox v0.5.3
	github.com/zenta-dev/zever/core/payment v0.5.3
	github.com/zenta-dev/zever/core/permission v0.5.3
	github.com/zenta-dev/zever/core/queue v0.5.3
	github.com/zenta-dev/zever/core/ratelimit v0.5.3
	github.com/zenta-dev/zever/core/router v0.5.3
	github.com/zenta-dev/zever/core/secrets v0.5.3
	github.com/zenta-dev/zever/core/session v0.5.3
	github.com/zenta-dev/zever/core/storage v0.5.3
	github.com/zenta-dev/zever/core/tenant v0.5.3
	github.com/zenta-dev/zever/core/webhook v0.5.3
	github.com/zenta-dev/zever/core/workflow v0.5.3
	github.com/zenta-dev/zever/dsl v0.5.3
	github.com/zenta-dev/zever/orm v0.5.3
	go.yaml.in/yaml/v3 v3.0.5
)

require (
	cel.dev/expr v0.25.3 // indirect
	charm.land/bubbles/v2 v2.0.0 // indirect
	charm.land/lipgloss/v2 v2.0.1 // indirect
	cloud.google.com/go v0.123.0 // indirect
	cloud.google.com/go/auth v0.24.0 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.3.0 // indirect
	cloud.google.com/go/compute/metadata v0.10.0 // indirect
	cloud.google.com/go/firestore v1.24.0 // indirect
	cloud.google.com/go/iam v1.12.0 // indirect
	cloud.google.com/go/longrunning v1.2.0 // indirect
	cloud.google.com/go/monitoring v1.30.0 // indirect
	cloud.google.com/go/storage v1.62.1 // indirect
	firebase.google.com/go/v4 v4.22.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp v1.35.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric v0.56.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/internal/resourcemapping v0.56.0 // indirect
	github.com/HugoSmits86/nativewebp v1.3.0 // indirect
	github.com/MicahParks/keyfunc v1.9.0 // indirect
	github.com/PaddleHQ/paddle-go-sdk/v5 v5.2.0 // indirect
	github.com/andybalholm/brotli v1.1.1 // indirect
	github.com/anthonynsimon/bild v0.17.1 // indirect
	github.com/anthropics/anthropic-sdk-go v1.78.0 // indirect
	github.com/atotto/clipboard v0.1.4 // indirect
	github.com/aws/aws-sdk-go-v2 v1.47.1 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.20 // indirect
	github.com/aws/aws-sdk-go-v2/config v1.33.6 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.20.6 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.20.1 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.5.4 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.8.4 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.5.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.19 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.11.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.14.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.20.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/s3 v1.114.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.10.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.38.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.43.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.51.1 // indirect
	github.com/aws/smithy-go v1.28.2 // indirect
	github.com/aymanbagabas/go-udiff v0.4.1 // indirect
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/bmatcuk/doublestar/v4 v4.6.1 // indirect
	github.com/bufbuild/protocompile v0.14.1 // indirect
	github.com/buger/jsonparser v1.1.2 // indirect
	github.com/casbin/casbin/v2 v2.135.0 // indirect
	github.com/casbin/govaluate v1.3.0 // indirect
	github.com/catppuccin/go v0.2.0 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/charmbracelet/colorprofile v0.4.3 // indirect
	github.com/charmbracelet/ultraviolet v0.0.0-20260811164956-006e29f97886 // indirect
	github.com/charmbracelet/x/ansi v0.11.8 // indirect
	github.com/charmbracelet/x/exp/golden v0.0.0-20251109135125-8916d276318f // indirect
	github.com/charmbracelet/x/exp/ordered v0.1.0 // indirect
	github.com/charmbracelet/x/exp/strings v0.0.0-20240722160745-212f7b056ed0 // indirect
	github.com/charmbracelet/x/term v0.2.2 // indirect
	github.com/charmbracelet/x/termios v0.1.1 // indirect
	github.com/charmbracelet/x/windows v0.2.2 // indirect
	github.com/chromedp/cdproto v0.0.0-20260714215040-dc233986426f // indirect
	github.com/chromedp/chromedp v0.16.0 // indirect
	github.com/chromedp/sysutil v1.1.0 // indirect
	github.com/clipperhouse/displaywidth v0.11.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/cloudflare/cloudflare-go/v7 v7.12.0 // indirect
	github.com/cncf/xds/go v0.0.0-20260202195803-dba9d589def2 // indirect
	github.com/coder/websocket v1.8.15 // indirect
	github.com/coreos/go-oidc/v3 v3.21.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.7 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/envoyproxy/go-control-plane/envoy v1.39.0 // indirect
	github.com/envoyproxy/protoc-gen-validate v1.3.3 // indirect
	github.com/felixge/httpsnoop v1.1.0 // indirect
	github.com/ggicci/httpin v0.20.3 // indirect
	github.com/ggicci/owl v0.8.2 // indirect
	github.com/go-jose/go-jose/v4 v4.1.4 // indirect
	github.com/go-json-experiment/json v0.0.0-20260623181947-01eb4420fa68 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-redsync/redsync/v4 v4.13.0 // indirect
	github.com/gobwas/httphead v0.1.0 // indirect
	github.com/gobwas/pool v0.2.1 // indirect
	github.com/gobwas/ws v1.4.0 // indirect
	github.com/goccy/go-json v0.10.6 // indirect
	github.com/gofiber/fiber/v2 v2.52.15 // indirect
	github.com/golang-jwt/jwt/v4 v4.5.2 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/golang/mock v1.6.0 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/s2a-go v0.1.10 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.22 // indirect
	github.com/googleapis/gax-go/v2 v2.26.2 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.30.0 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/invopop/jsonschema v0.14.0 // indirect
	github.com/jackc/pgio v1.0.0 // indirect
	github.com/jackc/pglogrepl v0.0.0-20261003132456-662581bb6bb6 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.11.0 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/klauspost/compress v1.20.0 // indirect
	github.com/lucasb-eyer/go-colorful v1.4.1 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/mattn/go-runewidth v0.0.27 // indirect
	github.com/meilisearch/meilisearch-go v0.36.3 // indirect
	github.com/mitchellh/hashstructure/v2 v2.0.2 // indirect
	github.com/molecule-man/go-brrr v1.0.1 // indirect
	github.com/muesli/cancelreader v0.2.2 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/oklog/ulid/v2 v2.1.2 // indirect
	github.com/openai/openai-go/v3 v3.70.0 // indirect
	github.com/pb33f/ordered-map/v2 v2.3.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/planetscale/vtprotobuf v0.6.1-0.20240319094008-0393e58bdf10 // indirect
	github.com/posthog/posthog-go v1.32.0 // indirect
	github.com/qdrant/go-client v1.19.3 // indirect
	github.com/redis/go-redis/v9 v9.22.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	github.com/rs/zerolog v1.35.1 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/sony/gobreaker/v2 v2.4.0 // indirect
	github.com/sony/gobreaker/v2/redis v0.0.0-20260207092134-fed8e9eb35f9 // indirect
	github.com/spiffe/go-spiffe/v2 v2.8.1 // indirect
	github.com/standard-webhooks/standard-webhooks/libraries v0.0.1 // indirect
	github.com/stripe/stripe-go/v82 v82.5.1 // indirect
	github.com/tidwall/gjson v1.19.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	github.com/twilio/twilio-go v1.31.2 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasthttp v1.74.0 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	github.com/zenta-dev/zever/adapters/media/ffmpeg v0.5.3 // indirect
	github.com/zenta-dev/zever/core/agent v0.5.3 // indirect
	github.com/zenta-dev/zever/core/billing v0.5.3 // indirect
	github.com/zenta-dev/zever/core/job v0.5.3 // indirect
	github.com/zenta-dev/zever/core/media v0.5.3 // indirect
	github.com/zenta-dev/zever/core/password v0.5.3 // indirect
	github.com/zenta-dev/zever/core/rag v0.5.3 // indirect
	github.com/zenta-dev/zever/core/resilience v0.5.3 // indirect
	github.com/zenta-dev/zever/core/scheduler v0.5.3 // indirect
	github.com/zenta-dev/zever/core/search v0.5.3 // indirect
	github.com/zenta-dev/zever/core/vectorstore v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/cas v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/codec v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/dbconn v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/endpoint v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/firebase v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/grpcclient v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/httpclient v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/kvstore v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/lrucache v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/mcpclient v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/msgspan v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/outboxbridge v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/providersclient v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/providersopt v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/redisclient v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/redisopt v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/registry v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/retry v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/s3opts v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/traceprop v0.5.3 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/detectors/gcp v1.45.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.70.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.70.0 // indirect
	go.opentelemetry.io/otel v1.46.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc v1.46.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.46.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.46.0 // indirect
	go.opentelemetry.io/otel/metric v1.46.0 // indirect
	go.opentelemetry.io/otel/sdk v1.46.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.46.0 // indirect
	go.opentelemetry.io/otel/trace v1.46.0 // indirect
	go.opentelemetry.io/proto/otlp v1.11.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.yaml.in/yaml/v4 v4.0.0-rc.2 // indirect
	golang.org/x/crypto v0.57.0 // indirect
	golang.org/x/image v0.45.0 // indirect
	golang.org/x/net v0.59.0 // indirect
	golang.org/x/oauth2 v0.37.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	golang.org/x/time v0.16.0 // indirect
	google.golang.org/api v0.300.0 // indirect
	google.golang.org/appengine/v2 v2.0.6 // indirect
	google.golang.org/genai v1.72.0 // indirect
	google.golang.org/genproto v0.0.0-20260715232425-e75dac1f907d // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260819154853-08b0e4226688 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260921155816-b14227669459 // indirect
	google.golang.org/grpc v1.85.0-dev.0.20260825072537-93e31b48545e // indirect
	google.golang.org/protobuf v1.36.12 // indirect
	googlemaps.github.io/maps v1.7.0 // indirect
	modernc.org/libc v1.77.1 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
	modernc.org/sqlite v1.60.1 // indirect
)

replace (
	github.com/zenta-dev/zever/adapters/ai/anthropic => ../../adapters/ai/anthropic
	github.com/zenta-dev/zever/adapters/ai/gemini => ../../adapters/ai/gemini
	github.com/zenta-dev/zever/adapters/ai/ollama => ../../adapters/ai/ollama
	github.com/zenta-dev/zever/adapters/ai/openai => ../../adapters/ai/openai
	github.com/zenta-dev/zever/adapters/analytics/log => ../../adapters/analytics/log
	github.com/zenta-dev/zever/adapters/analytics/posthog => ../../adapters/analytics/posthog
	github.com/zenta-dev/zever/adapters/auth/jwt => ../../adapters/auth/jwt
	github.com/zenta-dev/zever/adapters/auth/oidc => ../../adapters/auth/oidc
	github.com/zenta-dev/zever/adapters/auth/session => ../../adapters/auth/session
	github.com/zenta-dev/zever/adapters/billing/paddle => ../../adapters/billing/paddle
	github.com/zenta-dev/zever/adapters/billing/stripe => ../../adapters/billing/stripe
	github.com/zenta-dev/zever/adapters/billing/stub => ../../adapters/billing/stub
	github.com/zenta-dev/zever/adapters/cache/db => ../../adapters/cache/db
	github.com/zenta-dev/zever/adapters/cache/memory => ../../adapters/cache/memory
	github.com/zenta-dev/zever/adapters/cache/redis => ../../adapters/cache/redis
	github.com/zenta-dev/zever/adapters/cdn/cloudflare => ../../adapters/cdn/cloudflare
	github.com/zenta-dev/zever/adapters/cdn/noop => ../../adapters/cdn/noop
	github.com/zenta-dev/zever/adapters/crypto/local => ../../adapters/crypto/local
	github.com/zenta-dev/zever/adapters/db/postgres => ../../adapters/db/postgres
	github.com/zenta-dev/zever/adapters/db/sqlite => ../../adapters/db/sqlite
	github.com/zenta-dev/zever/adapters/document/latex => ../../adapters/document/latex
	github.com/zenta-dev/zever/adapters/document/local => ../../adapters/document/local
	github.com/zenta-dev/zever/adapters/document/remote => ../../adapters/document/remote
	github.com/zenta-dev/zever/adapters/eventbus/memory => ../../adapters/eventbus/memory
	github.com/zenta-dev/zever/adapters/eventbus/redis => ../../adapters/eventbus/redis
	github.com/zenta-dev/zever/adapters/flag/firebase => ../../adapters/flag/firebase
	github.com/zenta-dev/zever/adapters/flag/static => ../../adapters/flag/static
	github.com/zenta-dev/zever/adapters/geo/google => ../../adapters/geo/google
	github.com/zenta-dev/zever/adapters/geo/osm => ../../adapters/geo/osm
	github.com/zenta-dev/zever/adapters/geo/static => ../../adapters/geo/static
	github.com/zenta-dev/zever/adapters/i18n/embed => ../../adapters/i18n/embed
	github.com/zenta-dev/zever/adapters/i18n/remote => ../../adapters/i18n/remote
	github.com/zenta-dev/zever/adapters/idempotency/memory => ../../adapters/idempotency/memory
	github.com/zenta-dev/zever/adapters/idempotency/redis => ../../adapters/idempotency/redis
	github.com/zenta-dev/zever/adapters/lock/memory => ../../adapters/lock/memory
	github.com/zenta-dev/zever/adapters/lock/redis => ../../adapters/lock/redis
	github.com/zenta-dev/zever/adapters/log/noop => ../../adapters/log/noop
	github.com/zenta-dev/zever/adapters/log/pretty => ../../adapters/log/pretty
	github.com/zenta-dev/zever/adapters/log/slog => ../../adapters/log/slog
	github.com/zenta-dev/zever/adapters/log/zerolog => ../../adapters/log/zerolog
	github.com/zenta-dev/zever/adapters/mailer/log => ../../adapters/mailer/log
	github.com/zenta-dev/zever/adapters/mailer/smtp => ../../adapters/mailer/smtp
	github.com/zenta-dev/zever/adapters/media/local => ../../adapters/media/local
	github.com/zenta-dev/zever/adapters/media/s3 => ../../adapters/media/s3
	github.com/zenta-dev/zever/adapters/notification/fcm => ../../adapters/notification/fcm
	github.com/zenta-dev/zever/adapters/notification/log => ../../adapters/notification/log
	github.com/zenta-dev/zever/adapters/notification/twilio => ../../adapters/notification/twilio
	github.com/zenta-dev/zever/adapters/observability/noop => ../../adapters/observability/noop
	github.com/zenta-dev/zever/adapters/observability/otlp => ../../adapters/observability/otlp
	github.com/zenta-dev/zever/adapters/observability/stdout => ../../adapters/observability/stdout
	github.com/zenta-dev/zever/adapters/outbox/db => ../../adapters/outbox/db
	github.com/zenta-dev/zever/adapters/password/argon2 => ../../adapters/password/argon2
	github.com/zenta-dev/zever/adapters/payment/paddle => ../../adapters/payment/paddle
	github.com/zenta-dev/zever/adapters/payment/stripe => ../../adapters/payment/stripe
	github.com/zenta-dev/zever/adapters/payment/stub => ../../adapters/payment/stub
	github.com/zenta-dev/zever/adapters/permission/casbin => ../../adapters/permission/casbin
	github.com/zenta-dev/zever/adapters/permission/noop => ../../adapters/permission/noop
	github.com/zenta-dev/zever/adapters/permission/rbac => ../../adapters/permission/rbac
	github.com/zenta-dev/zever/adapters/queue/db => ../../adapters/queue/db
	github.com/zenta-dev/zever/adapters/queue/memory => ../../adapters/queue/memory
	github.com/zenta-dev/zever/adapters/queue/redis => ../../adapters/queue/redis
	github.com/zenta-dev/zever/adapters/ratelimit/memory => ../../adapters/ratelimit/memory
	github.com/zenta-dev/zever/adapters/ratelimit/redis => ../../adapters/ratelimit/redis
	github.com/zenta-dev/zever/adapters/router/fiber => ../../adapters/router/fiber
	github.com/zenta-dev/zever/adapters/router/stdhttp => ../../adapters/router/stdhttp
	github.com/zenta-dev/zever/adapters/scheduler/embedded => ../../adapters/scheduler/embedded
	github.com/zenta-dev/zever/adapters/search/db => ../../adapters/search/db
	github.com/zenta-dev/zever/adapters/search/meilisearch => ../../adapters/search/meilisearch
	github.com/zenta-dev/zever/adapters/secrets/env => ../../adapters/secrets/env
	github.com/zenta-dev/zever/adapters/session/memory => ../../adapters/session/memory
	github.com/zenta-dev/zever/adapters/session/redis => ../../adapters/session/redis
	github.com/zenta-dev/zever/adapters/storage/local => ../../adapters/storage/local
	github.com/zenta-dev/zever/adapters/storage/r2 => ../../adapters/storage/r2
	github.com/zenta-dev/zever/adapters/storage/s3 => ../../adapters/storage/s3
	github.com/zenta-dev/zever/adapters/tenant/header => ../../adapters/tenant/header
	github.com/zenta-dev/zever/adapters/tenant/single => ../../adapters/tenant/single
	github.com/zenta-dev/zever/adapters/vectorstore/db => ../../adapters/vectorstore/db
	github.com/zenta-dev/zever/adapters/vectorstore/qdrant => ../../adapters/vectorstore/qdrant
	github.com/zenta-dev/zever/adapters/webhook/http => ../../adapters/webhook/http
	github.com/zenta-dev/zever/adapters/webhook/queue => ../../adapters/webhook/queue
	github.com/zenta-dev/zever/adapters/workflow/db => ../../adapters/workflow/db
	github.com/zenta-dev/zever/adapters/workflow/memory => ../../adapters/workflow/memory
	github.com/zenta-dev/zever/core/ai => ../../core/ai
	github.com/zenta-dev/zever/core/analytics => ../../core/analytics
	github.com/zenta-dev/zever/core/auth => ../../core/auth
	github.com/zenta-dev/zever/core/cache => ../../core/cache
	github.com/zenta-dev/zever/core/cdn => ../../core/cdn
	github.com/zenta-dev/zever/core/crypto => ../../core/crypto
	github.com/zenta-dev/zever/core/db => ../../core/db
	github.com/zenta-dev/zever/core/document => ../../core/document
	github.com/zenta-dev/zever/core/eventbus => ../../core/eventbus
	github.com/zenta-dev/zever/core/flag => ../../core/flag
	github.com/zenta-dev/zever/core/geo => ../../core/geo
	github.com/zenta-dev/zever/core/i18n => ../../core/i18n
	github.com/zenta-dev/zever/core/idempotency => ../../core/idempotency
	github.com/zenta-dev/zever/core/job => ../../core/job
	github.com/zenta-dev/zever/core/lock => ../../core/lock
	github.com/zenta-dev/zever/core/log => ../../core/log
	github.com/zenta-dev/zever/core/mailer => ../../core/mailer
	github.com/zenta-dev/zever/core/notification => ../../core/notification
	github.com/zenta-dev/zever/core/observability => ../../core/observability
	github.com/zenta-dev/zever/core/outbox => ../../core/outbox
	github.com/zenta-dev/zever/core/payment => ../../core/payment
	github.com/zenta-dev/zever/core/permission => ../../core/permission
	github.com/zenta-dev/zever/core/queue => ../../core/queue
	github.com/zenta-dev/zever/core/ratelimit => ../../core/ratelimit
	github.com/zenta-dev/zever/core/resilience => ../../core/resilience
	github.com/zenta-dev/zever/core/router => ../../core/router
	github.com/zenta-dev/zever/core/secrets => ../../core/secrets
	github.com/zenta-dev/zever/core/session => ../../core/session
	github.com/zenta-dev/zever/core/storage => ../../core/storage
	github.com/zenta-dev/zever/core/tenant => ../../core/tenant
	github.com/zenta-dev/zever/core/webhook => ../../core/webhook
	github.com/zenta-dev/zever/core/workflow => ../../core/workflow
	github.com/zenta-dev/zever/dsl => ../../dsl
	github.com/zenta-dev/zever/orm => ../../orm
)

replace github.com/zenta-dev/zever/config => ../../config

replace github.com/zenta-dev/zever/container => ../../container

replace github.com/zenta-dev/zever/core/billing => ../../core/billing

replace github.com/zenta-dev/zever/core/media => ../../core/media

replace github.com/zenta-dev/zever/core/password => ../../core/password

replace github.com/zenta-dev/zever/core/scheduler => ../../core/scheduler

replace github.com/zenta-dev/zever/core/search => ../../core/search

replace github.com/zenta-dev/zever/core/vectorstore => ../../core/vectorstore

replace github.com/zenta-dev/zever/shared/cas => ../../shared/cas

replace github.com/zenta-dev/zever/shared/codec => ../../shared/codec

replace github.com/zenta-dev/zever/shared/dbconn => ../../shared/dbconn

replace github.com/zenta-dev/zever/shared/endpoint => ../../shared/endpoint

replace github.com/zenta-dev/zever/shared/firebase => ../../shared/firebase

replace github.com/zenta-dev/zever/shared/grpcclient => ../../shared/grpcclient

replace github.com/zenta-dev/zever/shared/httpclient => ../../shared/httpclient

replace github.com/zenta-dev/zever/shared/kvstore => ../../shared/kvstore

replace github.com/zenta-dev/zever/shared/lrucache => ../../shared/lrucache

replace github.com/zenta-dev/zever/shared/msgspan => ../../shared/msgspan

replace github.com/zenta-dev/zever/shared/outboxbridge => ../../shared/outboxbridge

replace github.com/zenta-dev/zever/shared/providersclient => ../../shared/providersclient

replace github.com/zenta-dev/zever/shared/providersopt => ../../shared/providersopt

replace github.com/zenta-dev/zever/shared/redisclient => ../../shared/redisclient

replace github.com/zenta-dev/zever/shared/redisopt => ../../shared/redisopt

replace github.com/zenta-dev/zever/shared/registry => ../../shared/registry

replace github.com/zenta-dev/zever/shared/retry => ../../shared/retry

replace github.com/zenta-dev/zever/shared/s3opts => ../../shared/s3opts

replace github.com/zenta-dev/zever/shared/traceprop => ../../shared/traceprop

replace github.com/zenta-dev/zever/adapters/media/ffmpeg => ../../adapters/media/ffmpeg

replace github.com/zenta-dev/zever/core/authz => ../../core/authz

replace github.com/zenta-dev/zever/core/middleware => ../../core/middleware

replace github.com/zenta-dev/zever/shared/apperror => ../../shared/apperror

replace github.com/zenta-dev/zever/core/agent => ../../core/agent

replace github.com/zenta-dev/zever/core/rag => ../../core/rag

replace github.com/zenta-dev/zever/shared/mcpclient => ../../shared/mcpclient

replace github.com/zenta-dev/zever/adapters/crypto/kms => ../../adapters/crypto/kms

replace github.com/zenta-dev/zever/adapters/idempotency/db => ../../adapters/idempotency/db

replace github.com/zenta-dev/zever/adapters/outbox/cdc => ../../adapters/outbox/cdc

replace github.com/zenta-dev/zever/adapters/outbox/memory => ../../adapters/outbox/memory

replace github.com/zenta-dev/zever/adapters/resilience/inproc => ../../adapters/resilience/inproc

replace github.com/zenta-dev/zever/adapters/resilience/redis => ../../adapters/resilience/redis

replace github.com/zenta-dev/zever/adapters/scheduler/postgres => ../../adapters/scheduler/postgres

replace github.com/zenta-dev/zever/adapters/secrets/vault => ../../adapters/secrets/vault

replace github.com/zenta-dev/zever/adapters/session/db => ../../adapters/session/db
