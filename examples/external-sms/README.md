# external-sms: third-party battery template

Copy-paste starter proving an out-of-repo author can ship a battery using
only public modules. Deliberately outside the workspace, deliberately NOT
`zenta-dev` path (`github.com/example/zever-sms`).

## Files

- `go.mod`: own module. `require`s pin `v0.0.0` with `replace =>
  ../../<path>` to this checkout for local dev. Published forks replace
  those with real versions.
- `sms.go`: full path in one package. `SMS` interface + `Options{From}` +
  `Validate`, string `Adapter` + `Register`/`Open` over `shared/registry`,
  deterministic stub driver, then plugin wiring: `RegisterStub` (adapter),
  `config.RegisterPluginValidator` (battery config decode), and
  `container.RegisterPlugin("sms", container.PluginAPIVersion,
  BuildFromConfig)` (container resolve reading `cfg.Plugins["sms"]`).
- `app_test.go`: end-to-end. Builds `config` with `Plugins["sms"]`,
  `container.New`, `Resolve`, `Send` via stub (asserts recorded message +
  per-container caching), plus a version-mismatch case asserting the
  registration error.

## Publish checklist

1. Copy this dir out of the checkout into YOUR repo.
2. In `go.mod`: keep the module path yours, drop the `replace`s, require
   real published zever versions.
3. Tag YOUR repo with semver (`vX.Y.Z`). No lockstep with zever needed.
4. Keep the stub deterministic and network-free so the test stays green.
