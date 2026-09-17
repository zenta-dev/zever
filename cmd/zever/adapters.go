package main

// This file blank-imports every adapter package selected by
// config.Default() so that `zever doctor` (and any other subcommand that
// touches container.Container) can actually resolve batteries instead of
// reporting "unknown adapter (forgotten import?)" for everything — a
// battery's registry only gains an adapter once its subpackage's init() has
// run, and nothing else in this binary otherwise references these packages.
//
// mailer has no zero-infra default adapter (config.Default() leaves it
// unset), so it isn't imported here.
//
// Removed from the zen-go original: db/mysql and router/chi have no zever
// counterpart (zever ships db/postgres, db/sqlite and router/fiber,
// router/stdhttp), so their blank imports are dropped here. The remaining
// entries are retargeted mechanical zen-go -> zever renames.
import (
	_ "github.com/zenta-dev/zever/ai/ollama"
	_ "github.com/zenta-dev/zever/analytics/log"
	_ "github.com/zenta-dev/zever/auth/jwt"
	_ "github.com/zenta-dev/zever/billing/stub"
	_ "github.com/zenta-dev/zever/cache/memory"
	_ "github.com/zenta-dev/zever/crypto/local"
	_ "github.com/zenta-dev/zever/db/postgres"
	_ "github.com/zenta-dev/zever/db/sqlite"
	_ "github.com/zenta-dev/zever/document/local"
	_ "github.com/zenta-dev/zever/eventbus/memory"
	_ "github.com/zenta-dev/zever/flag/static"
	_ "github.com/zenta-dev/zever/geo/static"
	_ "github.com/zenta-dev/zever/i18n/embed"
	_ "github.com/zenta-dev/zever/idempotency/memory"
	_ "github.com/zenta-dev/zever/lock/memory"
	_ "github.com/zenta-dev/zever/log/pretty"
	_ "github.com/zenta-dev/zever/media/local"
	_ "github.com/zenta-dev/zever/notification/log"
	_ "github.com/zenta-dev/zever/observability/stdout"
	_ "github.com/zenta-dev/zever/payment/stub"
	_ "github.com/zenta-dev/zever/permission/rbac"
	_ "github.com/zenta-dev/zever/queue/memory"
	_ "github.com/zenta-dev/zever/ratelimit/memory"
	_ "github.com/zenta-dev/zever/scheduler/embedded"
	_ "github.com/zenta-dev/zever/search/postgres"
	_ "github.com/zenta-dev/zever/secrets/env"
	_ "github.com/zenta-dev/zever/session/memory"
	_ "github.com/zenta-dev/zever/storage/local"
	_ "github.com/zenta-dev/zever/tenant/single"
	_ "github.com/zenta-dev/zever/vectorstore/sqlite"
	_ "github.com/zenta-dev/zever/webhook/http"
	_ "github.com/zenta-dev/zever/workflow/memory"
)
