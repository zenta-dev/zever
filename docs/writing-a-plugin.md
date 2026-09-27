# Writing a plugin

Two plugin shapes exist: a new adapter for an existing battery, and an
entirely new battery (`examples/sms` dogfoods the second shape end to end).

Conformance kits are the acceptance gate for both shapes: run the kit
before publishing. Three batteries ship kits today:
`github.com/zenta-dev/zever/core/cache/cachetest`,
`github.com/zenta-dev/zever/core/queue/queuetest`, and
`github.com/zenta-dev/zever/core/storage/storagetest`.

## New adapter for an existing battery

Batteries own a small interface plus a `Register`/`Open` pair. The container
is the sole resolver; adapter packages expose constructors and a `Register()`
function, never self-register (no `init` wiring, no globals). Hosts register
explicitly — each adapter module under `adapters/<battery>/<name>` wires
itself into its `core` registry with one `Register()` call before first use.
See `adapters/cache/memory/register.go` for the pattern and the generated
`app.go` for how hosts emit exactly the selected set.

Adapters are plain strings (`type Adapter string`, e.g.
`cache.Memory = "memory"`). A custom backend is just a new name —
`ParseAdapter` accepts any non-empty name, so it round-trips through config
file and env selection like any core adapter. `String()` is the identity
(`"unknown"` only for the empty value).

Worked example: a `cache` adapter in module `zever-adapter-foo`.

```go
package foo // import "example.com/zever-adapter-foo"

import (
	"context"
	"time"

	"github.com/zenta-dev/zever/core/cache"
)

// driver implements all 8 cache.Cache methods.
type driver struct{ /* connection state */ }

func (d *driver) Get(ctx context.Context, key string) ([]byte, error) { panic("implement") }
func (d *driver) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	panic("implement")
}
func (d *driver) SetIfAbsent(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	panic("implement")
}
func (d *driver) Delete(ctx context.Context, key string) error { panic("implement") }
func (d *driver) Increment(ctx context.Context, key string) error { panic("implement") }
func (d *driver) Decrement(ctx context.Context, key string) error { panic("implement") }
func (d *driver) Exists(ctx context.Context, key string) (bool, error) { panic("implement") }
func (d *driver) Close(ctx context.Context) error { panic("implement") }

// New matches cache.Factory.
func New(opts cache.Options) (cache.Cache, error) {
	return &driver{}, nil
}
```

Host wiring (your app, not the adapter package):

```go
const fooAdapter = cache.Adapter("foo")

if err := cache.Register(fooAdapter, foo.New); err != nil {
	return err
}

c, err := cache.Open(fooAdapter, cache.Options{})
if err != nil {
	return err
}
```

Via config and container, no extra code: `cache: {adapter: foo}` (or
`CACHE_ADAPTER=foo`) resolves through the registered factory on first
`container.Cache()` call, like any core adapter.

Rules:

- Do not add methods to the public battery interface; propose a new
  capability interface instead (e.g. `cache.CompareAndSwapCache`).
- `Open` calls `opts.Validate()` first; keep validation fail-closed and
  join all violations.
- Errors stay sentinel-based, `errors.Is`-compatible, prefixed with the
  battery name (`cache:`). Never leak secrets or raw option maps into
  error text; display via `config.RedactedServices`.
- `ctx` is the first arg for I/O and is never stored in a struct.
- Prove parity before publishing:

```go
import "github.com/zenta-dev/zever/core/cache/cachetest"

func TestConformance(t *testing.T) {
	t.Parallel()
	cachetest.Conformance(t, func(t *testing.T) cache.Cache {
		t.Helper()
		c, err := foo.New(cache.Options{})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = c.Close(t.Context()) })
		return c
	})
}
```

## Entirely new battery

Ship module `zever-battery-sms` mirroring the core battery shape —
interface, string `Adapter`, typed `Options` with `Validate`, `Register` /
`Open` over a factory registry, sentinel errors. Copy `examples/sms/sms.go`
(the `SMS` interface, `Stub` adapter, `Options{From}`, `Register`, `Open`)
and add a deterministic stub backend like `examples/sms/stub` (no network).

Config: entries live under `config.Config.Plugins`, keyed by plugin name
with raw options the plugin validates itself
(`map[string]Service[json.RawMessage]`, nil by default — initialize the
map before assigning):

```go
cfg.Plugins = map[string]config.Service[json.RawMessage]{
	"sms": {
		Adapter: "stub",
		Options: json.RawMessage(`{"from":"+15550000"}`),
	},
}
```

Register a validator so `Config.Validate` covers the plugin (unregistered
plugins skip validation; their options stay opaque):

```go
if err := config.RegisterPluginValidator("sms", func(adapter string, opts json.RawMessage) error {
	var o sms.Options
	if err := json.Unmarshal(opts, &o); err != nil {
		return err
	}
	return o.Validate()
}); err != nil {
	return err
}
```

Container: register a versioned build func once at startup, then resolve
per container like any core battery. The build receives the container's
`*config.Config` and usually decodes its `Plugins` entry:

```go
if err := container.RegisterPlugin[sms.SMS]("sms", container.PluginAPIVersion, func(cfg *config.Config) (sms.SMS, error) {
	var opts sms.Options
	if err := json.Unmarshal(cfg.Plugins["sms"].Options, &opts); err != nil {
		return nil, err
	}
	return sms.Open(sms.Stub, opts)
}); err != nil {
	return err
}

c := container.New(cfg)
svc, err := container.Resolve[sms.SMS](c, "sms")
```

Successful builds cache per `Container` (failures retry on next `Resolve`);
a stored value of the wrong type fails `Resolve` with a type-mismatch
error. Resolved plugins close with `Container.Close`. See
`examples/sms/app/main.go` for the full startup-to-`Send` flow.

## No-manifest third-party pattern

There is no plugin manifest file. Two artifacts you already ship are the
component list and the manifest.

- `go.mod` requires are the component list: each chosen adapter module
  (for example `github.com/zenta-dev/zever/adapters/cache/redis`) appears
  as a `require`, so the module graph contains exactly the selected set
  and nothing more. `zever new` emits one `require` per chosen nested
  adapter module for this reason.
- The generated `app.go` imports plus `Register()` calls are the manifest:
  one named import plus one `Register()` call per nested adapter module
  (for example `cachereids.Register()` for cache/redis), emitted exactly
  for the selected batteries (floor plus picks, nothing more). Verify by
  checking `zever.yaml` and `app.go` list the same batteries.

This mirrors the `database/sql` driver model described in
https://go.dev/doc/database/open-handle: drivers register a named factory
with the standard registry, and the host program pulls a driver into the
build by importing its package and invoking registration before opening it
by name. Import alone resolves nothing; registration without the `go.mod`
require does not build. Keep the same split: the adapter module registers
its name, the host decides which names exist in its binary.

## PluginAPIVersion contract

- The core publishes one integer, `container.PluginAPIVersion` (currently
  `1`). It bumps only on a breaking host-side change.
- `RegisterPlugin` rejects any other version, empty names, and nil builds;
  re-registering a name fails. Mismatches fail fast at registration, never
  silently at resolve time.
- Pre-1.0 `0.x` core may break plugin surface with release-note docs, per
  the repo's versioning rule. Pin scaffolds with `--framework-version`.

## Naming and publishing

- New adapter for an existing battery: `zever-adapter-<name>`
  (e.g. `zever-adapter-foo` for a `cache` backend).
- Entirely new battery: `zever-battery-<name>`
  (e.g. `zever-battery-sms`).
- Publish from your own repo with independent semver tags: the plugin is a
  standalone Go module, versioned on its own schedule, not a lockstep tag
  of this repo. Document user-facing changes under your own
  `CHANGELOG.md` `## [Unreleased]`; if the change touches this repo's
  scaffold or docs, add an entry here too.
- Gate every release on the `PluginAPIVersion` check: `RegisterPlugin`
  with a mismatched version must fail before publish, and the README must
  state which `PluginAPIVersion` the release targets.
- Conformance self-check is the acceptance proof: run the kit for the
  battery (`core/cache/cachetest` for cache,
  `core/queue/queuetest` for queue, `core/storage/storagetest` for
  storage) against your factory and link its passing run from the plugin
  README.
- Release both version pins together per the repo release checklist, and
  document user-facing changes under `CHANGELOG.md` `## [Unreleased]`.

## Lazy deps: register only what you choose

The scaffold floor is intentionally slim (`auth`, `db`, `log`, `router`,
`scheduler`). `log/slog` and `router/stdhttp` are stdlib-only and wired by
the container itself; `auth/jwt`, `db/sqlite` and `scheduler/embedded` are
nested modules the generated `app.go` imports and registers explicitly.
Everything else is opt-in, and the container never pulls a heavy client
into the build unless chosen:

- Light adapter (e.g. `queue/memory`, `permission/noop`): resolves via the
  container's own wiring. No import and no `Register` call needed.
- Nested-module adapter (e.g. `notification/fcm`, `cache/redis`,
  `db/sqlite`): import its module **and** call its `Register` before first
  use (e.g. `fcm.Register()`), and require the module in `go.mod`
  (`zever new` emits both). Adapter packages live under
  `adapters/<battery>/<name>` (e.g. `adapters/cache/redis`,
  `adapters/db/sqlite`). Import alone resolves nothing.
- The generated `app.go` emits exactly the selected set (floor + picks,
  nothing more) — verify by checking `zever.yaml` and `app.go` list the
  same batteries.
