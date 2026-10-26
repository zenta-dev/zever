module github.com/zenta-dev/zever/cmd/zever

go 1.27.3

require (
	charm.land/bubbletea/v2 v2.1.0
	charm.land/huh/v2 v2.0.3
	github.com/charmbracelet/x/exp/teatest/v2 v2.0.0-20260924144451-d676b019604b
	github.com/fsnotify/fsnotify v1.10.1
	github.com/spf13/cobra v1.10.2
	github.com/spf13/pflag v1.0.10
	github.com/traefik/yaegi v0.16.1
	github.com/zenta-dev/zever/adapters/ai/anthropic v0.8.0
	github.com/zenta-dev/zever/adapters/analytics/log v0.8.0
	github.com/zenta-dev/zever/adapters/auth/jwt v0.8.0
	github.com/zenta-dev/zever/adapters/billing/stub v0.8.0
	github.com/zenta-dev/zever/adapters/cache/memory v0.8.0
	github.com/zenta-dev/zever/adapters/cdn/noop v0.8.0
	github.com/zenta-dev/zever/adapters/crypto/local v0.8.0
	github.com/zenta-dev/zever/adapters/db/postgres v0.8.0
	github.com/zenta-dev/zever/adapters/db/sqlite v0.8.0
	github.com/zenta-dev/zever/adapters/document/local v0.8.0
	github.com/zenta-dev/zever/adapters/eventbus/memory v0.8.0
	github.com/zenta-dev/zever/adapters/flag/static v0.8.0
	github.com/zenta-dev/zever/adapters/geo/static v0.8.0
	github.com/zenta-dev/zever/adapters/i18n/embed v0.8.0
	github.com/zenta-dev/zever/adapters/idempotency/memory v0.8.0
	github.com/zenta-dev/zever/adapters/lock/memory v0.8.0
	github.com/zenta-dev/zever/adapters/log/slog v0.8.0
	github.com/zenta-dev/zever/adapters/mailer/log v0.8.0
	github.com/zenta-dev/zever/adapters/media/local v0.8.0
	github.com/zenta-dev/zever/adapters/notification/log v0.8.0
	github.com/zenta-dev/zever/adapters/observability/stdout v0.8.0
	github.com/zenta-dev/zever/adapters/outbox/db v0.8.0
	github.com/zenta-dev/zever/adapters/password/argon2 v0.8.0
	github.com/zenta-dev/zever/adapters/payment/stub v0.8.0
	github.com/zenta-dev/zever/adapters/permission/noop v0.8.0
	github.com/zenta-dev/zever/adapters/queue/memory v0.8.0
	github.com/zenta-dev/zever/adapters/ratelimit/memory v0.8.0
	github.com/zenta-dev/zever/adapters/resilience/inproc v0.8.0
	github.com/zenta-dev/zever/adapters/router/stdhttp v0.8.0
	github.com/zenta-dev/zever/adapters/scheduler/embedded v0.8.0
	github.com/zenta-dev/zever/adapters/search/db v0.8.0
	github.com/zenta-dev/zever/adapters/secrets/env v0.8.0
	github.com/zenta-dev/zever/adapters/session/memory v0.8.0
	github.com/zenta-dev/zever/adapters/storage/local v0.8.0
	github.com/zenta-dev/zever/adapters/tenant/single v0.8.0
	github.com/zenta-dev/zever/adapters/vectorstore/db v0.8.0
	github.com/zenta-dev/zever/adapters/webhook/http v0.8.0
	github.com/zenta-dev/zever/adapters/workflow/memory v0.8.0
	github.com/zenta-dev/zever/config v0.8.0
	github.com/zenta-dev/zever/container v0.8.0
	github.com/zenta-dev/zever/core/ai v0.8.0
	github.com/zenta-dev/zever/core/analytics v0.8.0
	github.com/zenta-dev/zever/core/auth v0.8.0
	github.com/zenta-dev/zever/core/billing v0.8.0
	github.com/zenta-dev/zever/core/cache v0.8.0
	github.com/zenta-dev/zever/core/cdn v0.8.0
	github.com/zenta-dev/zever/core/crypto v0.8.0
	github.com/zenta-dev/zever/core/db v0.8.0
	github.com/zenta-dev/zever/core/document v0.8.0
	github.com/zenta-dev/zever/core/eventbus v0.8.0
	github.com/zenta-dev/zever/core/flag v0.8.0
	github.com/zenta-dev/zever/core/geo v0.8.0
	github.com/zenta-dev/zever/core/i18n v0.8.0
	github.com/zenta-dev/zever/core/idempotency v0.8.0
	github.com/zenta-dev/zever/core/lock v0.8.0
	github.com/zenta-dev/zever/core/log v0.8.0
	github.com/zenta-dev/zever/core/mailer v0.8.0
	github.com/zenta-dev/zever/core/media v0.8.0
	github.com/zenta-dev/zever/core/notification v0.8.0
	github.com/zenta-dev/zever/core/observability v0.8.0
	github.com/zenta-dev/zever/core/outbox v0.8.0
	github.com/zenta-dev/zever/core/password v0.8.0
	github.com/zenta-dev/zever/core/payment v0.8.0
	github.com/zenta-dev/zever/core/permission v0.8.0
	github.com/zenta-dev/zever/core/queue v0.8.0
	github.com/zenta-dev/zever/core/ratelimit v0.8.0
	github.com/zenta-dev/zever/core/resilience v0.8.0
	github.com/zenta-dev/zever/core/router v0.8.0
	github.com/zenta-dev/zever/core/scheduler v0.8.0
	github.com/zenta-dev/zever/core/search v0.8.0
	github.com/zenta-dev/zever/core/secrets v0.8.0
	github.com/zenta-dev/zever/core/session v0.8.0
	github.com/zenta-dev/zever/core/storage v0.8.0
	github.com/zenta-dev/zever/core/tenant v0.8.0
	github.com/zenta-dev/zever/core/vectorstore v0.8.0
	github.com/zenta-dev/zever/core/webhook v0.8.0
	github.com/zenta-dev/zever/core/workflow v0.8.0
	github.com/zenta-dev/zever/dsl v0.8.0
	github.com/zenta-dev/zever/orm v0.8.0
	github.com/zenta-dev/zever/serve v0.8.0
	github.com/zenta-dev/zever/shared/codec v0.8.0
	github.com/zenta-dev/zever/shared/dbconn v0.8.0
	// test-only: the YAML parser reads the GitHub-Actions workflow the
	// tests check. Shipped code parses no configuration file (v0.8.0 is
	// env-only), so no non-test file may import a YAML or TOML parser;
	// the no-config-parser rule of tools/tagcheck enforces that.
	go.yaml.in/yaml/v3 v3.0.5
	golang.org/x/mod v0.42.0
)

require (
	charm.land/bubbles/v2 v2.0.0 // indirect
	charm.land/lipgloss/v2 v2.0.1 // indirect
	cloud.google.com/go/compute/metadata v0.10.0 // indirect
	github.com/HugoSmits86/nativewebp v1.3.0 // indirect
	github.com/anthonynsimon/bild v0.17.1 // indirect
	github.com/anthropics/anthropic-sdk-go v1.82.0 // indirect
	github.com/atotto/clipboard v0.1.4 // indirect
	github.com/aws/aws-sdk-go-v2 v1.47.3 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.22 // indirect
	github.com/aws/aws-sdk-go-v2/config v1.33.9 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.20.9 // indirect
	github.com/aymanbagabas/go-udiff v0.4.1 // indirect
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/bufbuild/protocompile v0.14.1 // indirect
	github.com/buger/jsonparser v1.1.2 // indirect
	github.com/catppuccin/go v0.2.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/charmbracelet/colorprofile v0.4.3 // indirect
	github.com/charmbracelet/ultraviolet v0.0.0-20261008173134-6b8d4baf91b4 // indirect
	github.com/charmbracelet/x/ansi v0.11.9 // indirect
	github.com/charmbracelet/x/exp/golden v0.0.0-20251109135125-8916d276318f // indirect
	github.com/charmbracelet/x/exp/ordered v0.1.0 // indirect
	github.com/charmbracelet/x/exp/strings v0.0.0-20240722160745-212f7b056ed0 // indirect
	github.com/charmbracelet/x/term v0.2.2 // indirect
	github.com/charmbracelet/x/termios v0.1.1 // indirect
	github.com/charmbracelet/x/windows v0.2.2 // indirect
	github.com/chromedp/cdproto v0.157.8 // indirect
	github.com/chromedp/chromedp v0.21.0 // indirect
	github.com/clipperhouse/displaywidth v0.11.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.7 // indirect
	github.com/dustin/go-humanize v1.1.0 // indirect
	github.com/go-json-experiment/json v0.0.0-20260213210345-44df1a37e875 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/invopop/jsonschema v0.14.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.11.0 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/lucasb-eyer/go-colorful v1.4.1 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/mattn/go-runewidth v0.0.30 // indirect
	github.com/mitchellh/hashstructure/v2 v2.0.2 // indirect
	github.com/muesli/cancelreader v0.2.2 // indirect
	github.com/ncruces/go-strftime v1.1.0 // indirect
	github.com/oklog/ulid/v2 v2.1.2 // indirect
	github.com/pb33f/ordered-map/v2 v2.3.1 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/sony/gobreaker/v2 v2.4.0 // indirect
	github.com/standard-webhooks/standard-webhooks/libraries v0.0.1 // indirect
	github.com/tidwall/gjson v1.19.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	github.com/xo/terminfo v1.0.0 // indirect
	github.com/zenta-dev/zever/adapters/log/noop v0.8.0 // indirect
	github.com/zenta-dev/zever/adapters/media/ffmpeg v0.8.0 // indirect
	github.com/zenta-dev/zever/core/agent v0.8.0 // indirect
	github.com/zenta-dev/zever/core/authz v0.8.0 // indirect
	github.com/zenta-dev/zever/core/job v0.8.0 // indirect
	github.com/zenta-dev/zever/core/middleware v0.8.0 // indirect
	github.com/zenta-dev/zever/core/rag v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/endpoint v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/grpcclient v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/httpclient v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/lrucache v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/mcpclient v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/msgspan v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/outboxbridge v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/providersopt v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/redisopt v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/ref v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/registry v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/retry v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/traceprop v0.8.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.70.0 // indirect
	go.opentelemetry.io/otel v1.47.0 // indirect
	go.opentelemetry.io/otel/log v1.47.0 // indirect
	go.opentelemetry.io/otel/metric v1.47.0 // indirect
	go.opentelemetry.io/otel/sdk v1.47.0 // indirect
	go.opentelemetry.io/otel/trace v1.47.0 // indirect
	go.yaml.in/yaml/v4 v4.0.0-rc.2 // indirect
	golang.org/x/crypto v0.58.0 // indirect
	golang.org/x/image v0.45.0 // indirect
	golang.org/x/net v0.61.0 // indirect
	golang.org/x/oauth2 v0.37.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	google.golang.org/api v0.304.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20261005182115-fad411399dd8 // indirect
	google.golang.org/grpc v1.85.0-dev.0.20260825072537-93e31b48545e // indirect
	google.golang.org/protobuf v1.36.12 // indirect
	modernc.org/libc v1.79.0 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
	modernc.org/sqlite v1.61.0 // indirect
)
