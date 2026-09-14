package casbin

import (
	"context"
	"errors"
	"fmt"
	"sync"

	casbinlib "github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"

	"github.com/zenta-dev/zever/permission"
)

// defaultModelText is the embedded deny-override RBAC model. Role rules
// seeded from Options use obj "*" (RBAC rules ignore the resource), so the
// matcher accepts a wildcard policy object alongside exact matches.
const defaultModelText = `[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act, eft

[role_definition]
g = _, _

[policy_effect]
e = some(where (p.eft == allow)) && !some(where (p.eft == deny))

[matchers]
m = (p.obj == "*" || r.obj == p.obj) && g(r.sub, p.sub) && r.act == p.act
`

// checker is a permission.Checker backed by a Casbin synced enforcer.
type checker struct {
	mu         sync.Mutex
	e          *casbinlib.SyncedEnforcer
	roles      map[string][]string
	persistent map[string]bool
	groupRefs  map[string]int
	pending    [][]string
}

// New builds a permission.Checker from opts. When ModelPath and PolicyPath
// are both set the enforcer loads them; otherwise an embedded deny-override
// model is used and Rules/Roles are seeded into it.
func New(opts permission.Options) (permission.Checker, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("casbin: invalid options: %w", err)
	}

	if opts.ModelPath != "" {
		e, err := casbinlib.NewSyncedEnforcer(opts.ModelPath, opts.PolicyPath)
		if err != nil {
			return nil, fmt.Errorf("casbin: creating enforcer: %w", err)
		}

		return newChecker(e, opts.Roles)
	}

	// defaultModelText is a const and no adapter is configured on this path,
	// so model parsing and enforcer creation below are infallible by
	// construction; the file-loaded path above stays the fallible path that
	// surfaces creation failures (fail-closed preserved).
	m, _ := model.NewModelFromString(defaultModelText)
	e, _ := casbinlib.NewSyncedEnforcer(m)

	for _, r := range opts.Rules {
		eft := string(r.Effect)
		if eft == "" {
			eft = string(permission.Allow)
		}

		// Seeding uses fixed 4-field rules against the const model with no
		// adapter or dispatcher configured, so AddPolicy cannot fail here
		// (the p assertion always exists); only duplicates are reachable.
		ok, _ := e.AddPolicy(r.Role, "*", r.Action, eft)

		if !ok {
			return nil, &DuplicatePolicyError{Role: r.Role, Action: r.Action}
		}
	}

	persistent := make(map[string]bool)

	for user, roles := range opts.Roles {
		for _, role := range roles {
			// Fixed 2-element groupings against the const g definition with
			// an infallible default role manager cannot fail; repeats are
			// harmless (the allowlist entry is recorded exactly once).
			_, _ = e.AddGroupingPolicy(user, role)

			persistent[user+":"+role] = true
		}
	}

	return &checker{
		e:          e,
		roles:      opts.Roles,
		persistent: persistent,
		groupRefs:  make(map[string]int),
	}, nil
}

// newChecker wraps a file-loaded enforcer, recording its existing groupings
// as persistent so per-request cleanup never removes them.
func newChecker(e *casbinlib.SyncedEnforcer, roles map[string][]string) (permission.Checker, error) {
	if e == nil {
		return nil, ErrNilEnforcer
	}

	persistent := make(map[string]bool)

	if groups, err := e.GetGroupingPolicy(); err == nil {
		for _, g := range groups {
			if len(g) >= 2 {
				persistent[g[0]+":"+g[1]] = true
			}
		}
	}

	return &checker{
		e:          e,
		roles:      roles,
		persistent: persistent,
		groupRefs:  make(map[string]int),
	}, nil
}

// Can reports whether subject may perform action on resource. Enforcement
// failures are fail-closed: they return a zero Decision with the error.
func (c *checker) Can(ctx context.Context, subject permission.Subject, action string, resource permission.Resource) (permission.Decision, error) {
	_ = ctx

	added, err := c.prepareGroupings(subject)
	if err != nil {
		if cerr := c.cleanupGroupings(added); cerr != nil {
			return permission.Decision{}, errors.Join(err, cerr)
		}

		return permission.Decision{}, err
	}

	obj := resource.Type + ":" + resource.ID

	ok, explain, err := c.e.EnforceEx(subject.ID, obj, action)
	if err != nil {
		if cerr := c.cleanupGroupings(added); cerr != nil {
			return permission.Decision{}, errors.Join(fmt.Errorf("casbin: enforce: %w", err), cerr)
		}

		return permission.Decision{}, fmt.Errorf("casbin: enforce: %w", err)
	}

	if cerr := c.cleanupGroupings(added); cerr != nil {
		return permission.Decision{Allowed: ok}, cerr
	}

	if ok {
		return permission.Decision{Allowed: true, Reason: "allow"}, nil
	}

	for _, field := range explain {
		if field == string(permission.Deny) {
			return permission.Decision{Allowed: false, Reason: "explicit_deny"}, nil
		}
	}

	return permission.Decision{Allowed: false, Reason: "implicit_deny"}, nil
}

// prepareGroupings adds transient groupings for the subject's allowlisted
// roles. Claimed roles outside the Options.Roles allowlist are skipped, so a
// subject cannot escalate privilege by asserting arbitrary roles.
func (c *checker) prepareGroupings(subject permission.Subject) ([][]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.retryPending(); err != nil {
		return nil, err
	}

	var added [][]string

	for _, role := range subject.Roles {
		if !c.isAuthorizedRole(subject.ID, role) {
			continue
		}

		key := subject.ID + ":" + role

		if c.persistent[key] {
			continue
		}

		if c.groupRefs[key] > 0 {
			c.groupRefs[key]++

			added = append(added, []string{subject.ID, role})

			continue
		}

		has, err := c.e.HasGroupingPolicy(subject.ID, role)
		if err != nil {
			return added, fmt.Errorf("casbin: has grouping policy: %w", err)
		}

		if !has {
			// Has was checked just above under the same held mutex and every
			// enforcer mutation goes through this mutex, so no concurrent
			// add can win this race: Add reports ok == true here.
			if _, err := c.e.AddGroupingPolicy(subject.ID, role); err != nil {
				return added, fmt.Errorf("casbin: add grouping policy: %w", err)
			}
		}

		c.groupRefs[key] = 1

		added = append(added, []string{subject.ID, role})
	}

	return added, nil
}

// cleanupGroupings removes transient groupings added by prepareGroupings.
// Removal failures are queued in pending (capped at 1000) for a later retry
// and returned joined.
func (c *checker) cleanupGroupings(added [][]string) error {
	if len(added) == 0 {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	var toRemove [][]string

	for _, g := range added {
		if len(g) < 2 {
			continue
		}

		key := g[0] + ":" + g[1]

		if c.persistent[key] {
			continue
		}

		cnt, ok := c.groupRefs[key]
		if !ok || cnt <= 0 {
			continue
		}

		cnt--

		if cnt == 0 {
			delete(c.groupRefs, key)

			toRemove = append(toRemove, g)
		} else {
			c.groupRefs[key] = cnt
		}
	}

	var errs []error

	for _, g := range toRemove {
		if err := c.removeGroupingPolicy(g); err != nil {
			errs = append(errs, err)

			c.groupRefs[g[0]+":"+g[1]] = 1
		}
	}

	if len(errs) > 0 {
		remaining := 1000 - len(c.pending)
		if remaining > 0 {
			queued := toRemove
			if len(queued) > remaining {
				queued = queued[:remaining]
			}

			c.pending = append(c.pending, queued...)
		}

		return fmt.Errorf("casbin: cleanup grouping policies: %w", errors.Join(errs...))
	}

	return nil
}

// isAuthorizedRole reports whether role is allowlisted for subjectID.
// Callers must hold c.mu.
func (c *checker) isAuthorizedRole(subjectID, role string) bool {
	for _, r := range c.roles[subjectID] {
		if r == role {
			return true
		}
	}

	return false
}

// retryPending re-attempts queued grouping removals. Callers must hold c.mu.
func (c *checker) retryPending() error {
	if len(c.pending) == 0 {
		return nil
	}

	var remaining [][]string

	var errs []error

	for _, p := range c.pending {
		if err := c.removeGroupingPolicy(p); err != nil {
			remaining = append(remaining, p)
			errs = append(errs, err)
		} else if len(p) >= 2 {
			delete(c.groupRefs, p[0]+":"+p[1])
		}
	}

	c.pending = remaining

	if len(errs) > 0 {
		return fmt.Errorf("casbin: retry pending grouping removal: %w", errors.Join(errs...))
	}

	return nil
}

// removeGroupingPolicy removes one grouping policy, converting panics
// (Casbin panics on malformed policy arguments) into errors.
func (c *checker) removeGroupingPolicy(g []string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("casbin: panic removing grouping policy %v: %v", g, r)
		}
	}()

	args := make([]interface{}, len(g))
	for i, v := range g {
		args[i] = v
	}

	_, err = c.e.RemoveGroupingPolicy(args...)
	if err != nil {
		err = fmt.Errorf("casbin: remove grouping policy %v: %w", g, err)
	}

	return err
}
