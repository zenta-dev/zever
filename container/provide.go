package container

import (
	"fmt"
	"reflect"
	"runtime"
	"sort"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/shared/ref"
)

// refKey is the map key form of a ref.Ref: the config service key plus the
// instance name. It is the identity pools, diagnostics, and Close all
// attribute to: two Refs with equal Service and Name denote the same
// instance.
type refKey struct {
	service string
	name    string
}

// String renders the key as the diagnostics label "service" for the
// default instance or "service/name" otherwise. It carries no option or
// secret content, only the address.
func (k refKey) String() string {
	if k.name == "" || k.name == ref.DefaultName {
		return k.service
	}

	return k.service + "/" + k.name
}

// Provider is one explicitly wired service instance: a ref.Ref plus the
// factory that builds it from its Options. Providers are constructed only
// by Provide, Provide1, Provide2, and Provide3, which capture the
// declaration site for error messages.
//
// Provider is sealed: its single method is unexported, so only this
// package can implement it and a hand-rolled Provider never compiles
// (see testdata/compilefail/sealed).
type Provider interface {
	sealProvider()
}

// Option customizes one Provide call. Options are per-call values:
// Lazy, Eager, and Optional toggle the provider's own behavior, while
// WithOptions carries a typed applier for that call's Options only.
type Option interface {
	sealOption()
}

// providerConfig is the folded form of one Provide call's Options.
type providerConfig struct {
	eager    bool
	optional bool
	appliers []func(any)
}

// providerOptions folds Option values into a providerConfig.
type providerOptions struct {
	cfg providerConfig
}

// sealOption marks providerOptions as an Option implementation.
func (providerOptions) sealOption() {}

// Lazy marks the provider resolved on first Get: nothing builds at New
// and nothing builds until the instance (or one of its dependents) is
// resolved. This is the default; Lazy exists so call sites can state it.
func Lazy() Option {
	return providerOptions{}
}

// Eager marks the provider built during New, in declaration order after
// graph validation. A failed eager build is recorded against the Ref and
// surfaced by every later Get of it; unrelated Refs still resolve.
func Eager() Option {
	return providerOptions{cfg: providerConfig{eager: true}}
}

// Optional marks every dependency of this Provide call optional: a dep
// with no provider resolves to its type's zero value instead of failing
// the graph. Use it for best-effort injects (telemetry, dedupe stores)
// that must never block the dependent.
func Optional() Option {
	return providerOptions{cfg: providerConfig{optional: true}}
}

// optionApplier carries one WithOptions func into the fold.
type optionApplier struct {
	apply func(any)
}

// sealOption marks optionApplier as an Option implementation.
func (optionApplier) sealOption() {}

// WithOptions appends fn to the Options mutations applied to the resolved
// Options value before the factory runs. Resolution seeds a fresh value
// from the service defaults, overlays the process environment, and only
// then runs appliers, so explicit Provide-site options win over env. It is
// generic over O so the compiler checks fn against the Provide call's
// Options type; an applier whose type does not match the resolved Options
// is ignored.
func WithOptions[O any](fn func(*O)) Option {
	return optionApplier{apply: func(v any) {
		if p, ok := v.(*O); ok {
			fn(p)
		}
	}}
}

// foldOptions merges one Provide call's options. Later toggles win; every
// WithOptions applier is kept in order.
func foldOptions(opts []Option) providerConfig {
	var cfg providerConfig

	for _, opt := range opts {
		switch o := opt.(type) {
		case providerOptions:
			if o.cfg.eager {
				cfg.eager = true
			}

			if o.cfg.optional {
				cfg.optional = true
			}
		case optionApplier:
			cfg.appliers = append(cfg.appliers, o.apply)
		}
	}

	return cfg
}

// provider is the type-erased form of one Provide call. The typed factory
// and dependency zero values are captured in closures so resolution never
// type-asserts across battery boundaries.
type provider struct {
	key      refKey
	site     string
	deps     []refKey
	optional bool
	eager    bool
	newOpts  func() any
	seedOpts func(opts, seed any)
	apply    func(any)
	zeroDeps []func() any
	build    func(opts any, depVals []any) (any, error)
}

// sealProvider marks provider as the sole Provider implementation.
func (p *provider) sealProvider() {}

// callSite captures the Provide caller's file:line for error messages.
// The skip is fixed: callSite <- Provide* <- declaration site.
func callSite() string {
	_, file, line, ok := runtime.Caller(2)
	if !ok {
		return "unknown"
	}

	return fmt.Sprintf("%s:%d", file, line)
}

// makeProvider folds opts, captures the declaration site, and stores the
// type-erased build closures around the typed factory.
func makeProvider[T, O any](
	r ref.Ref[T, O],
	deps []refKey,
	zeroDeps []func() any,
	build func(opts O, depVals []any) (T, error),
	opts []Option,
	site string,
) Provider {
	cfg := foldOptions(opts)

	return &provider{
		key:      refKey{service: r.Service, name: r.Name},
		site:     site,
		deps:     deps,
		optional: cfg.optional,
		eager:    cfg.eager,
		newOpts: func() any {
			o := new(O)

			return o
		},
		seedOpts: func(opts, seed any) {
			o, ok := opts.(*O)
			if !ok {
				return
			}

			s, ok := seed.(O)
			if !ok {
				return
			}

			*o = s
		},
		apply: func(v any) {
			for _, fn := range cfg.appliers {
				fn(v)
			}
		},
		zeroDeps: zeroDeps,
		build: func(optsAny any, depVals []any) (any, error) {
			o, ok := optsAny.(*O)
			if !ok {
				var zero T

				return zero, fmt.Errorf("container: %s: options type mismatch", refKey{service: r.Service, name: r.Name})
			}

			return build(*o, depVals)
		},
	}
}

// Provide wires one service instance: the Ref r resolves to factory(opts),
// where opts is a fresh Options value mutated by any WithOptions. The
// declaration site is captured for graph error messages. Instance names
// must not contain "__": it separates the service, name, and field in env
// keys (see config.ApplyEnv).
func Provide[T, O any](r ref.Ref[T, O], factory func(O) (T, error), opts ...Option) Provider {
	return makeProvider(r, nil, nil, func(o O, _ []any) (T, error) {
		return factory(o)
	}, opts, callSite())
}

// Provide1 wires one service instance with one dependency: factory
// receives opts plus the resolved d1 instance. When the provider is
// Optional and d1 has no provider, factory receives d1's zero value.
func Provide1[T, O, D1, OD1 any](
	r ref.Ref[T, O],
	d1 ref.Ref[D1, OD1],
	factory func(O, D1) (T, error),
	opts ...Option,
) Provider {
	deps := []refKey{{service: d1.Service, name: d1.Name}}
	zeroDeps := []func() any{func() any {
		var zero D1

		return zero
	}}

	return makeProvider(r, deps, zeroDeps, func(o O, d []any) (T, error) {
		v1, ok := d[0].(D1)
		if !ok {
			var zero T

			return zero, fmt.Errorf("container: %s: dependency %s type mismatch", refKey{service: r.Service, name: r.Name}, deps[0])
		}

		return factory(o, v1)
	}, opts, callSite())
}

// Provide2 wires one service instance with two dependencies, in order. See
// Provide1 for the resolution and Optional contract.
func Provide2[T, O, D1, OD1, D2, OD2 any](
	r ref.Ref[T, O],
	d1 ref.Ref[D1, OD1],
	d2 ref.Ref[D2, OD2],
	factory func(O, D1, D2) (T, error),
	opts ...Option,
) Provider {
	deps := []refKey{{service: d1.Service, name: d1.Name}, {service: d2.Service, name: d2.Name}}
	zeroDeps := []func() any{
		func() any {
			var zero D1

			return zero
		},
		func() any {
			var zero D2

			return zero
		},
	}

	return makeProvider(r, deps, zeroDeps, func(o O, d []any) (T, error) {
		v1, ok := d[0].(D1)
		if !ok {
			var zero T

			return zero, fmt.Errorf("container: %s: dependency %s type mismatch", refKey{service: r.Service, name: r.Name}, deps[0])
		}

		v2, ok := d[1].(D2)
		if !ok {
			var zero T

			return zero, fmt.Errorf("container: %s: dependency %s type mismatch", refKey{service: r.Service, name: r.Name}, deps[1])
		}

		return factory(o, v1, v2)
	}, opts, callSite())
}

// Provide3 wires one service instance with three dependencies, in order.
// See Provide1 for the resolution and Optional contract.
func Provide3[T, O, D1, OD1, D2, OD2, D3, OD3 any](
	r ref.Ref[T, O],
	d1 ref.Ref[D1, OD1],
	d2 ref.Ref[D2, OD2],
	d3 ref.Ref[D3, OD3],
	factory func(O, D1, D2, D3) (T, error),
	opts ...Option,
) Provider {
	deps := []refKey{
		{service: d1.Service, name: d1.Name},
		{service: d2.Service, name: d2.Name},
		{service: d3.Service, name: d3.Name},
	}
	zeroDeps := []func() any{
		func() any {
			var zero D1

			return zero
		},
		func() any {
			var zero D2

			return zero
		},
		func() any {
			var zero D3

			return zero
		},
	}

	return makeProvider(r, deps, zeroDeps, func(o O, d []any) (T, error) {
		v1, ok := d[0].(D1)
		if !ok {
			var zero T

			return zero, fmt.Errorf("container: %s: dependency %s type mismatch", refKey{service: r.Service, name: r.Name}, deps[0])
		}

		v2, ok := d[1].(D2)
		if !ok {
			var zero T

			return zero, fmt.Errorf("container: %s: dependency %s type mismatch", refKey{service: r.Service, name: r.Name}, deps[1])
		}

		v3, ok := d[2].(D3)
		if !ok {
			var zero T

			return zero, fmt.Errorf("container: %s: dependency %s type mismatch", refKey{service: r.Service, name: r.Name}, deps[2])
		}

		return factory(o, v1, v2, v3)
	}, opts, callSite())
}

// WrapNoErr adapts a factory without an error return to the
// func(O) (T, error) shape Provide takes. Use it for the infallible leaf
// constructors (for example log pretty/slog/zerolog New).
func WrapNoErr[T, O any](build func(O) T) func(O) (T, error) {
	return func(o O) (T, error) {
		return build(o), nil
	}
}

// WrapNoOpts adapts a factory without options to the func(O) (T, error)
// shape Provide takes. The Options value is accepted and ignored; use it
// for the option-free constructors (for example log/noop.New,
// observability/noop.New).
func WrapNoOpts[T, O any](build func() T) func(O) (T, error) {
	return func(_ O) (T, error) {
		return build(), nil
	}
}

// provEntry is one cached provider resolution: the built value, or the
// in-flight build waiters park on. A failed build is never cached: the key
// is evicted so the next resolution retries.
type provEntry struct {
	val      any
	building bool
	ready    chan struct{}
}

// validateProviders checks the full graph at construction: duplicate Refs,
// missing non-Optional dependencies, and dependency cycles. Every finding
// is recorded against the dependent Ref so Get surfaces it at resolution;
// validation itself never opens anything.
func validateProviders(defs []*provider) map[refKey]error {
	errs := make(map[refKey]error)
	sites := make(map[refKey]string)

	for _, d := range defs {
		if first, ok := sites[d.key]; ok {
			errs[d.key] = DuplicateRefError{
				Service: d.key.service,
				Name:    d.key.name,
				First:   first,
				Second:  d.site,
			}

			continue
		}

		sites[d.key] = d.site
	}

	for _, d := range defs {
		if _, bad := errs[d.key]; bad {
			continue
		}

		for _, dep := range d.deps {
			if _, ok := sites[dep]; !ok && !d.optional {
				errs[d.key] = NotWiredError{
					Service:   dep.service,
					Name:      dep.name,
					Dependent: d.key.String(),
					Site:      d.site,
					Fix:       "add a Provide call for " + dep.String() + " to the same New call, or mark the dependent Optional",
				}

				break
			}
		}
	}

	markCycle := func(path []refKey) {
		for _, k := range path {
			if _, bad := errs[k]; bad {
				continue
			}

			errs[k] = DependencyCycleError{
				Service: k.service,
				Name:    k.name,
				Path:    keyStrings(path),
			}
		}
	}

	visited := make(map[refKey]bool)
	inStack := make(map[refKey]bool)
	byKey := make(map[refKey]*provider)

	for _, d := range defs {
		if _, bad := errs[d.key]; !bad {
			byKey[d.key] = d
		}
	}

	var visit func(k refKey, stack []refKey)
	visit = func(k refKey, stack []refKey) {
		if visited[k] || errs[k] != nil {
			return
		}

		if inStack[k] {
			start := 0

			for i, s := range stack {
				if s == k {
					start = i

					break
				}
			}

			markCycle(append(append([]refKey(nil), stack[start:]...), k))

			return
		}

		d, ok := byKey[k]
		if !ok {
			return
		}

		inStack[k] = true
		stack = append(stack, k)

		for _, dep := range d.deps {
			if _, ok := byKey[dep]; ok {
				visit(dep, stack)
			}
		}

		delete(inStack, k)
		visited[k] = true
	}

	for _, d := range defs {
		visit(d.key, nil)
	}

	return errs
}

// keyStrings renders keys as service[/name] labels.
func keyStrings(keys []refKey) []string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k.String())
	}

	return out
}

// resolveProvider returns the cached instance for key, building it (and
// its dependencies, depth-first) on first use. Concurrent callers share
// one in-flight build; a failed build is evicted so the next call retries.
func (c *Container) resolveProvider(key refKey) (any, error) {
	if err, bad := c.provErrs[key]; bad {
		return nil, err
	}

	def, ok := c.provDefs[key]
	if !ok {
		return nil, NotWiredError{
			Service: key.service,
			Name:    key.name,
			Fix:     "pass container.Provide(" + key.String() + ", factory) to container.New",
		}
	}

	for {
		c.provMu.Lock()

		e, ok := c.provVals[key]
		if !ok {
			e = &provEntry{building: true, ready: make(chan struct{})}
			c.provVals[key] = e
			c.provMu.Unlock()

			val, err := c.buildProvider(def)
			c.provMu.Lock()

			if err != nil {
				delete(c.provVals, key)
				c.provMu.Unlock()
				close(e.ready)

				return nil, err
			}

			e.val = val
			e.building = false
			c.provMu.Unlock()
			close(e.ready)

			return val, nil
		}

		if e.building {
			ready := e.ready
			c.provMu.Unlock()
			<-ready

			continue
		}

		val := e.val
		c.provMu.Unlock()

		return val, nil
	}
}

// buildProvider resolves def's dependencies depth-first, seeds a fresh
// Options value from the service defaults overlaid with the process
// environment (see New), runs its WithOptions appliers, and runs the
// factory. A missing Optional dependency resolves to its zero value.
// Refs for unknown services skip the env overlay and start from zero O.
func (c *Container) buildProvider(def *provider) (any, error) {
	if c.envErr != nil {
		return nil, fmt.Errorf("container: %s: resolve env: %w", def.key, c.envErr)
	}

	depVals := make([]any, len(def.deps))

	for i, dk := range def.deps {
		if _, ok := c.provDefs[dk]; !ok && def.optional {
			depVals[i] = def.zeroDeps[i]()

			continue
		}

		v, err := c.resolveProvider(dk)
		if err != nil {
			return nil, err
		}

		depVals[i] = v
	}

	opts := def.newOpts()
	if seed := config.DefaultOptions(def.key.service, def.key.name); seed != nil {
		def.seedOpts(opts, seed)

		// The default instance reads the bare SERVICE_* namespace;
		// named instances read SERVICE__NAME__*.
		name := def.key.name
		if name == ref.DefaultName {
			name = ""
		}

		// The overlay applies to struct Options (every battery's are).
		// Non-struct Options skip it; WithOptions still applies below.
		if rv := reflect.ValueOf(opts); rv.Kind() == reflect.Pointer && !rv.IsNil() && rv.Elem().Kind() == reflect.Struct {
			if err := config.ApplyEnv(c.env, def.key.service, name, opts); err != nil {
				return nil, fmt.Errorf("container: %s: %w", def.key, err)
			}
		}
	}

	def.apply(opts)

	return def.build(opts, depVals)
}

// Get resolves the wired instance for r. With zero providers it returns a
// NotWiredError; a Ref whose construction failed validation (duplicate,
// missing dep, cycle) returns that error; a factory failure returns the
// factory's error without caching, so the next Get retries.
func Get[T, O any](c *Container, r ref.Ref[T, O]) (T, error) {
	var zero T

	if c == nil {
		return zero, NotWiredError{
			Service: r.Service,
			Name:    r.Name,
			Fix:     "build a Container with container.New before resolving",
		}
	}

	v, err := c.resolveProvider(refKey{service: r.Service, name: r.Name})
	if err != nil {
		return zero, err
	}

	out, ok := v.(T)
	if !ok {
		return zero, fmt.Errorf("container: %s: provider built %T, want %T", refKey{service: r.Service, name: r.Name}, v, zero)
	}

	return out, nil
}

// Wired lists every Ref passed to New as a service[/name] label, in sorted
// order: "db" for a default instance, "db/analytics" for a named one. Refs
// that failed graph validation are omitted: they can never resolve.
func (c *Container) Wired() []string {
	var out []string

	for _, k := range c.provOrder {
		if _, bad := c.provErrs[k]; bad {
			continue
		}

		out = append(out, k.String())
	}

	sort.Strings(out)

	return out
}

// providedDefault resolves the default Ref for one legacy service when a
// provider was wired for it. It reports ok=false when no provider exists,
// in which case the caller falls back to the legacy config path. When a
// provider exists, ok is true and err carries the resolution result.
func (c *Container) providedDefault[T, O any](r ref.Ref[T, O]) (T, bool, error) {
	var zero T

	k := refKey{service: r.Service, name: r.Name}
	if _, ok := c.provDefs[k]; !ok {
		return zero, false, nil
	}

	v, err := Get(c, r)

	return v, true, err
}

// providerCloseOrder returns the provOrder keys in dependency-first order
// so Close can walk them back-to-front (dependents before dependencies).
func (c *Container) providerCloseOrder() []refKey {
	visited := make(map[refKey]bool, len(c.provOrder))

	var order []refKey

	var visit func(k refKey)
	visit = func(k refKey) {
		if visited[k] {
			return
		}

		visited[k] = true

		if def, ok := c.provDefs[k]; ok {
			for _, dep := range def.deps {
				visit(dep)
			}
		}

		order = append(order, k)
	}

	for _, k := range c.provOrder {
		visit(k)
	}

	return order
}

// providerSnapshots returns resolved provided instances in reverse
// dependency order for Close: dependents shut down before the pools and
// services they borrowed. Each snapshot is labeled service[/name] for
// error reporting, never carrying resolved values into logs.
func (c *Container) providerSnapshots() []closeSnapshot {
	order := c.providerCloseOrder()

	c.provMu.Lock()
	defer c.provMu.Unlock()

	out := make([]closeSnapshot, 0, len(order))

	for i := len(order) - 1; i >= 0; i-- {
		if e, ok := c.provVals[order[i]]; ok && !e.building {
			out = append(out, closeSnapshot{v: e.val, ok: true})
		}
	}

	return out
}

// providerSnapshotNames parallels providerSnapshots order.
func (c *Container) providerSnapshotNames() []string {
	order := c.providerCloseOrder()

	c.provMu.Lock()
	defer c.provMu.Unlock()

	var out []string

	for i := len(order) - 1; i >= 0; i-- {
		if e, ok := c.provVals[order[i]]; ok && !e.building {
			out = append(out, order[i].String())
		}
	}

	return out
}
