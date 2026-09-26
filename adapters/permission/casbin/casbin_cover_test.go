package casbin

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	casbinlib "github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"

	"github.com/zenta-dev/zever/core/permission"
)

// glessModelText is a model without a role definition: every grouping
// operation against it fails with "missing required section g".
const glessModelText = `[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act, eft

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = r.sub == p.sub && r.obj == p.obj && r.act == p.act
`

// brokenMatcherModelText keeps the g definition but references an undefined
// matcher function, so EnforceEx always returns an error.
const brokenMatcherModelText = `[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act, eft

[role_definition]
g = _, _

[policy_effect]
e = some(where (p.eft == allow)) && !some(where (p.eft == deny))

[matchers]
m = notAFunction(r.sub, p.sub) && r.act == p.act
`

// errAdapter is a casbin persist.Adapter that fails grouping writes on
// demand to exercise checker error paths.
type errAdapter struct {
	addErr    error
	addRole   string
	removeErr error
}

func (a *errAdapter) LoadPolicy(model.Model) error { return nil }

func (a *errAdapter) SavePolicy(model.Model) error { return nil }

func (a *errAdapter) AddPolicy(sec string, _ string, rule []string) error {
	if a.addErr != nil && sec == "g" && (a.addRole == "" || (len(rule) > 1 && rule[1] == a.addRole)) {
		return a.addErr
	}

	return nil
}

func (a *errAdapter) RemovePolicy(sec string, _ string, _ []string) error {
	if a.removeErr != nil && sec == "g" {
		return a.removeErr
	}

	return nil
}

func (a *errAdapter) RemoveFilteredPolicy(sec string, _ string, _ int, _ ...string) error {
	if a.removeErr != nil && sec == "g" {
		return a.removeErr
	}

	return nil
}

// mustEnforcer builds a synced enforcer from model text.
func mustEnforcer(t *testing.T, text string) *casbinlib.SyncedEnforcer {
	t.Helper()

	m, err := model.NewModelFromString(text)
	if err != nil {
		t.Fatalf("model: %v", err)
	}

	e, err := casbinlib.NewSyncedEnforcer(m)
	if err != nil {
		t.Fatalf("enforcer: %v", err)
	}

	return e
}

// asChecker unwraps the concrete white-box checker.
func asChecker(t *testing.T, c permission.Checker) *checker {
	t.Helper()

	cc, ok := c.(*checker)
	if !ok {
		t.Fatalf("checker type = %T, want *checker", c)
	}

	return cc
}

// mustSeededChecker builds a checker through New for white-box tests.
func mustSeededChecker(t *testing.T, opts permission.Options) *checker {
	t.Helper()

	c, err := New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	return asChecker(t, c)
}

// transientChecker returns a checker with no persistent groupings and no
// seeded enforcer groupings, so every authorized role takes the transient
// prepare/cleanup path.
func transientChecker(t *testing.T, roles map[string][]string) *checker {
	t.Helper()

	c, err := newChecker(mustEnforcer(t, defaultModelText), roles)
	if err != nil {
		t.Fatalf("newChecker: %v", err)
	}

	return asChecker(t, c)
}

// seedAllow adds an admin/read allow rule, failing the test on error.
func seedAllow(t *testing.T, c *checker) {
	t.Helper()

	ok, err := c.e.AddPolicy("admin", "*", "read", "allow")
	if err != nil || !ok {
		t.Fatalf("AddPolicy = (%v, %v), want (true, nil)", ok, err)
	}
}

func TestCoverNewEnforcerCreateError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	modelPath := filepath.Join(dir, "model.conf")
	policyPath := filepath.Join(dir, "policy.csv")

	if err := os.WriteFile(modelPath, []byte("not a model"), 0o600); err != nil {
		t.Fatalf("write model: %v", err)
	}

	if err := os.WriteFile(policyPath, []byte("p, admin, *, read, allow\n"), 0o600); err != nil {
		t.Fatalf("write policy: %v", err)
	}

	_, err := New(permission.Options{ModelPath: modelPath, PolicyPath: policyPath})
	if err == nil {
		t.Fatal("New = nil error, want enforcer creation error")
	}

	if !strings.Contains(err.Error(), "casbin: creating enforcer") {
		t.Fatalf("err = %q, want substring %q", err, "casbin: creating enforcer")
	}
}

func TestCoverDuplicatePolicyErrorString(t *testing.T) {
	t.Parallel()

	err := DuplicatePolicyError{Role: "admin", Action: "read"}.Error()
	want := `casbin: duplicate policy: role "admin" action "read"`

	if err != want {
		t.Fatalf("Error() = %q, want %q", err, want)
	}

	if !errors.Is(DuplicatePolicyError{Role: "x", Action: "y"}, ErrDuplicatePolicy) {
		t.Fatal("errors.Is(DuplicatePolicyError, ErrDuplicatePolicy) = false, want true")
	}
}

func TestCoverNewCheckerGroupingError(t *testing.T) {
	t.Parallel()

	c, err := newChecker(mustEnforcer(t, glessModelText), map[string][]string{"alice": {"admin"}})
	if err != nil {
		t.Fatalf("newChecker: %v", err)
	}

	if got := len(asChecker(t, c).persistent); got != 0 {
		t.Fatalf("persistent entries = %d, want 0 after grouping read failure", got)
	}
}

func TestCoverCanPrepareErrorAlone(t *testing.T) {
	t.Parallel()

	c := transientChecker(t, map[string][]string{"carol": {"admin"}})
	seedAllow(t, c)
	c.e.SetAdapter(&errAdapter{addErr: errors.New("injected add failure")})

	_, err := c.Can(t.Context(), permission.Subject{ID: "carol", Roles: []string{"admin"}}, "read", permission.Resource{Type: "doc", ID: "1"})
	if err == nil {
		t.Fatal("Can = nil error, want prepare error")
	}

	if !strings.Contains(err.Error(), "casbin: add grouping policy") {
		t.Fatalf("err = %q, want substring %q", err, "casbin: add grouping policy")
	}

	if strings.Contains(err.Error(), "cleanup") {
		t.Fatalf("err = %q, want no cleanup failure", err)
	}
}

func TestCoverCanPrepareErrorJoinsCleanupError(t *testing.T) {
	t.Parallel()

	c := transientChecker(t, map[string][]string{"carol": {"admin", "broken"}})
	seedAllow(t, c)
	c.e.SetAdapter(&errAdapter{addErr: errors.New("injected add failure"), addRole: "broken", removeErr: errors.New("injected remove failure")})

	_, err := c.Can(t.Context(), permission.Subject{ID: "carol", Roles: []string{"admin", "broken"}}, "read", permission.Resource{Type: "doc", ID: "1"})
	if err == nil {
		t.Fatal("Can = nil error, want joined error")
	}

	for _, want := range []string{"casbin: add grouping policy", "casbin: cleanup grouping policies"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %q, want substring %q", err, want)
		}
	}
}

func TestCoverCanEnforceError(t *testing.T) {
	t.Parallel()

	c, err := newChecker(mustEnforcer(t, brokenMatcherModelText), map[string][]string{"alice": {"admin"}})
	if err != nil {
		t.Fatalf("newChecker: %v", err)
	}

	d, err := c.Can(t.Context(), permission.Subject{ID: "alice", Roles: []string{"admin"}}, "read", permission.Resource{Type: "doc", ID: "1"})
	if err == nil {
		t.Fatal("Can = nil error, want enforce error")
	}

	if !strings.Contains(err.Error(), "casbin: enforce:") {
		t.Fatalf("err = %q, want substring %q", err, "casbin: enforce:")
	}

	if d.Allowed {
		t.Fatalf("Decision = %+v, want zero decision on enforce failure", d)
	}
}

func TestCoverCanEnforceErrorJoinsCleanupError(t *testing.T) {
	t.Parallel()

	cc := asChecker(t, mustCheckerFrom(t, brokenMatcherModelText))
	cc.e.SetAdapter(&errAdapter{removeErr: errors.New("injected remove failure")})

	_, err := cc.Can(t.Context(), permission.Subject{ID: "alice", Roles: []string{"admin"}}, "read", permission.Resource{Type: "doc", ID: "1"})
	if err == nil {
		t.Fatal("Can = nil error, want joined error")
	}

	for _, want := range []string{"casbin: enforce:", "casbin: cleanup grouping policies"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %q, want substring %q", err, want)
		}
	}
}

// mustCheckerFrom builds a checker over model text with alice allowlisted
// for admin.
func mustCheckerFrom(t *testing.T, text string) permission.Checker {
	t.Helper()

	c, err := newChecker(mustEnforcer(t, text), map[string][]string{"alice": {"admin"}})
	if err != nil {
		t.Fatalf("newChecker: %v", err)
	}

	return c
}

func TestCoverCanCleanupErrorAlone(t *testing.T) {
	t.Parallel()

	c := transientChecker(t, map[string][]string{"alice": {"admin"}})
	seedAllow(t, c)
	c.e.SetAdapter(&errAdapter{removeErr: errors.New("injected remove failure")})

	d, err := c.Can(t.Context(), permission.Subject{ID: "alice", Roles: []string{"admin"}}, "read", permission.Resource{Type: "doc", ID: "1"})
	if err == nil {
		t.Fatal("Can = nil error, want cleanup error")
	}

	if !strings.Contains(err.Error(), "casbin: cleanup grouping policies") {
		t.Fatalf("err = %q, want substring %q", err, "casbin: cleanup grouping policies")
	}

	if !d.Allowed {
		t.Fatalf("Decision = %+v, want Allowed preserved alongside cleanup error", d)
	}
}

func TestCoverPrepareGroupingHasError(t *testing.T) {
	t.Parallel()

	c, err := newChecker(mustEnforcer(t, glessModelText), map[string][]string{"alice": {"admin"}})
	if err != nil {
		t.Fatalf("newChecker: %v", err)
	}

	_, err = c.Can(t.Context(), permission.Subject{ID: "alice", Roles: []string{"admin"}}, "read", permission.Resource{Type: "doc", ID: "1"})
	if err == nil {
		t.Fatal("Can = nil error, want has-grouping error")
	}

	if !strings.Contains(err.Error(), "casbin: has grouping policy") {
		t.Fatalf("err = %q, want substring %q", err, "casbin: has grouping policy")
	}
}

func TestCoverPrepareHasTrueSkip(t *testing.T) {
	t.Parallel()

	c := mustSeededChecker(t, allowOpts())
	delete(c.persistent, "alice:admin")

	added, err := c.prepareGroupings(permission.Subject{ID: "alice", Roles: []string{"admin"}})
	if err != nil {
		t.Fatalf("prepareGroupings: %v", err)
	}

	if len(added) != 1 || added[0][0] != "alice" || added[0][1] != "admin" {
		t.Fatalf("added = %v, want [[alice admin]] via existing grouping", added)
	}

	if err := c.cleanupGroupings(added); err != nil {
		t.Fatalf("cleanupGroupings: %v", err)
	}

	if has, err := c.e.HasGroupingPolicy("alice", "admin"); err != nil || has {
		t.Fatalf("HasGroupingPolicy = (%v, %v), want (false, nil) after cleanup", has, err)
	}
}

func TestCoverPrepareRetryPendingError(t *testing.T) {
	t.Parallel()

	c := asChecker(t, mustCheckerFrom(t, glessModelText))
	c.pending = [][]string{{"stale", "admin"}}

	_, err := c.prepareGroupings(permission.Subject{ID: "alice", Roles: []string{"admin"}})
	if err == nil {
		t.Fatal("prepareGroupings = nil error, want pending retry error")
	}

	if !strings.Contains(err.Error(), "casbin: retry pending grouping removal") {
		t.Fatalf("err = %q, want substring %q", err, "casbin: retry pending grouping removal")
	}
}

func TestCoverPrepareRefcountConcurrent(t *testing.T) {
	t.Parallel()

	c := transientChecker(t, map[string][]string{"alice": {"admin"}})
	seedAllow(t, c)

	if ok, err := c.e.AddGroupingPolicy("alice", "admin"); err != nil || !ok {
		t.Fatalf("AddGroupingPolicy = (%v, %v), want (true, nil)", ok, err)
	}

	// Simulate an in-flight holder so the grouping survives every cleanup
	// during the run: each prepare then takes the refcount++ branch and each
	// enforce still sees the grouping, keeping the test deterministic.
	c.groupRefs["alice:admin"] = 1

	const n = 16

	ctx := t.Context()
	start := make(chan struct{})

	var wg sync.WaitGroup

	errs := make([]error, n)
	allowed := make([]bool, n)

	for i := range n {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()
			<-start

			d, err := c.Can(ctx, permission.Subject{ID: "alice", Roles: []string{"admin"}}, "read", permission.Resource{Type: "doc", ID: "1"})
			errs[i] = err
			allowed[i] = d.Allowed
		}(i)
	}

	close(start)
	wg.Wait()

	for i := range n {
		if errs[i] != nil {
			t.Fatalf("goroutine %d: %v", i, errs[i])
		}

		if !allowed[i] {
			t.Fatalf("goroutine %d: denied, want allowed", i)
		}
	}

	if got := c.groupRefs["alice:admin"]; got != 1 {
		t.Fatalf("groupRefs = %d, want 1 (only the simulated holder remains)", got)
	}

	if err := c.cleanupGroupings([][]string{{"alice", "admin"}}); err != nil {
		t.Fatalf("drain cleanup: %v", err)
	}

	if len(c.groupRefs) != 0 {
		t.Fatalf("groupRefs = %v, want empty after drain", c.groupRefs)
	}

	if has, err := c.e.HasGroupingPolicy("alice", "admin"); err != nil || has {
		t.Fatalf("HasGroupingPolicy = (%v, %v), want (false, nil) after drain", has, err)
	}
}

func TestCoverCleanupBranches(t *testing.T) {
	t.Parallel()

	t.Run("empty", func(t *testing.T) {
		t.Parallel()

		if err := transientChecker(t, nil).cleanupGroupings(nil); err != nil {
			t.Fatalf("cleanupGroupings(nil) = %v, want nil", err)
		}
	})

	t.Run("short", func(t *testing.T) {
		t.Parallel()

		if err := transientChecker(t, nil).cleanupGroupings([][]string{{"solo"}}); err != nil {
			t.Fatalf("cleanupGroupings(short) = %v, want nil", err)
		}
	})

	t.Run("persistent", func(t *testing.T) {
		t.Parallel()

		c := mustSeededChecker(t, allowOpts())

		if err := c.cleanupGroupings([][]string{{"alice", "admin"}}); err != nil {
			t.Fatalf("cleanupGroupings(persistent) = %v, want nil", err)
		}

		if has, err := c.e.HasGroupingPolicy("alice", "admin"); err != nil || !has {
			t.Fatalf("HasGroupingPolicy = (%v, %v), want (true, nil)", has, err)
		}
	})

	t.Run("unknown", func(t *testing.T) {
		t.Parallel()

		if err := transientChecker(t, nil).cleanupGroupings([][]string{{"ghost", "admin"}}); err != nil {
			t.Fatalf("cleanupGroupings(unknown) = %v, want nil", err)
		}
	})

	t.Run("zero refcount", func(t *testing.T) {
		t.Parallel()

		c := transientChecker(t, nil)
		c.groupRefs["alice:admin"] = 0

		if err := c.cleanupGroupings([][]string{{"alice", "admin"}}); err != nil {
			t.Fatalf("cleanupGroupings(zero) = %v, want nil", err)
		}
	})

	t.Run("decrement keep", func(t *testing.T) {
		t.Parallel()

		c := transientChecker(t, nil)
		c.groupRefs["alice:admin"] = 2

		if err := c.cleanupGroupings([][]string{{"alice", "admin"}}); err != nil {
			t.Fatalf("cleanupGroupings = %v, want nil", err)
		}

		if got := c.groupRefs["alice:admin"]; got != 1 {
			t.Fatalf("groupRefs = %d, want 1", got)
		}
	})

	t.Run("removal error queued", func(t *testing.T) {
		t.Parallel()

		c := transientChecker(t, map[string][]string{"alice": {"admin"}})

		if ok, err := c.e.AddGroupingPolicy("alice", "admin"); err != nil || !ok {
			t.Fatalf("AddGroupingPolicy = (%v, %v), want (true, nil)", ok, err)
		}

		c.e.SetAdapter(&errAdapter{removeErr: errors.New("injected remove failure")})
		c.groupRefs["alice:admin"] = 1

		err := c.cleanupGroupings([][]string{{"alice", "admin"}})
		if err == nil {
			t.Fatal("cleanupGroupings = nil error, want removal error")
		}

		if !strings.Contains(err.Error(), "casbin: cleanup grouping policies") {
			t.Fatalf("err = %q, want substring %q", err, "casbin: cleanup grouping policies")
		}

		if got := c.groupRefs["alice:admin"]; got != 1 {
			t.Fatalf("groupRefs = %d, want 1 (restored after failure)", got)
		}

		if len(c.pending) != 1 {
			t.Fatalf("pending = %d entries, want 1 queued", len(c.pending))
		}
	})

	t.Run("pending cap", func(t *testing.T) {
		t.Parallel()

		c := transientChecker(t, map[string][]string{"alice": {"admin"}})

		if ok, err := c.e.AddGroupingPolicy("alice", "admin"); err != nil || !ok {
			t.Fatalf("AddGroupingPolicy = (%v, %v), want (true, nil)", ok, err)
		}

		c.e.SetAdapter(&errAdapter{removeErr: errors.New("injected remove failure")})
		c.groupRefs["alice:admin"] = 1
		c.pending = make([][]string, 1000)

		for i := range c.pending {
			c.pending[i] = []string{"u", "r"}
		}

		if err := c.cleanupGroupings([][]string{{"alice", "admin"}}); err == nil {
			t.Fatal("cleanupGroupings = nil error, want removal error")
		}

		if len(c.pending) != 1000 {
			t.Fatalf("pending = %d entries, want capped at 1000", len(c.pending))
		}
	})

	t.Run("pending truncated", func(t *testing.T) {
		t.Parallel()

		c := transientChecker(t, map[string][]string{"alice": {"admin", "viewer"}})

		for _, role := range []string{"admin", "viewer"} {
			if ok, err := c.e.AddGroupingPolicy("alice", role); err != nil || !ok {
				t.Fatalf("AddGroupingPolicy(%s) = (%v, %v), want (true, nil)", role, ok, err)
			}

			c.groupRefs["alice:"+role] = 1
		}

		c.e.SetAdapter(&errAdapter{removeErr: errors.New("injected remove failure")})
		c.pending = make([][]string, 999)

		for i := range c.pending {
			c.pending[i] = []string{"u", "r"}
		}

		err := c.cleanupGroupings([][]string{{"alice", "admin"}, {"alice", "viewer"}})
		if err == nil {
			t.Fatal("cleanupGroupings = nil error, want removal error")
		}

		if len(c.pending) != 1000 {
			t.Fatalf("pending = %d entries, want truncated to 1000", len(c.pending))
		}
	})
}

func TestCoverRetryPending(t *testing.T) {
	t.Parallel()

	t.Run("empty", func(t *testing.T) {
		t.Parallel()

		if err := transientChecker(t, nil).retryPending(); err != nil {
			t.Fatalf("retryPending() = %v, want nil", err)
		}
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		c := mustSeededChecker(t, allowOpts())
		c.pending = [][]string{{"alice", "admin"}}
		c.groupRefs["alice:admin"] = 1

		if err := c.retryPending(); err != nil {
			t.Fatalf("retryPending() = %v, want nil", err)
		}

		if len(c.pending) != 0 {
			t.Fatalf("pending = %v, want empty after success", c.pending)
		}

		if len(c.groupRefs) != 0 {
			t.Fatalf("groupRefs = %v, want empty after success", c.groupRefs)
		}

		if has, err := c.e.HasGroupingPolicy("alice", "admin"); err != nil || has {
			t.Fatalf("HasGroupingPolicy = (%v, %v), want (false, nil)", has, err)
		}
	})

	t.Run("failure", func(t *testing.T) {
		t.Parallel()

		c := asChecker(t, mustCheckerFrom(t, glessModelText))
		c.pending = [][]string{{"stale", "admin"}}

		err := c.retryPending()
		if err == nil {
			t.Fatal("retryPending() = nil error, want removal error")
		}

		if !strings.Contains(err.Error(), "casbin: retry pending grouping removal") {
			t.Fatalf("err = %q, want substring %q", err, "casbin: retry pending grouping removal")
		}

		if len(c.pending) != 1 {
			t.Fatalf("pending = %d entries, want failed entry retained", len(c.pending))
		}
	})
}

func TestCoverRemoveGroupingPolicy(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		c := mustSeededChecker(t, allowOpts())

		if err := c.removeGroupingPolicy([]string{"alice", "admin"}); err != nil {
			t.Fatalf("removeGroupingPolicy = %v, want nil", err)
		}

		if has, err := c.e.HasGroupingPolicy("alice", "admin"); err != nil || has {
			t.Fatalf("HasGroupingPolicy = (%v, %v), want (false, nil)", has, err)
		}
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		c := asChecker(t, mustCheckerFrom(t, glessModelText))

		err := c.removeGroupingPolicy([]string{"alice", "admin"})
		if err == nil {
			t.Fatal("removeGroupingPolicy = nil error, want removal error")
		}

		if !strings.Contains(err.Error(), "casbin: remove grouping policy") {
			t.Fatalf("err = %q, want substring %q", err, "casbin: remove grouping policy")
		}
	})

	t.Run("panic", func(t *testing.T) {
		t.Parallel()

		c := mustSeededChecker(t, allowOpts())

		err := c.removeGroupingPolicy([]string{})
		if err == nil {
			t.Fatal("removeGroupingPolicy = nil error, want panic recovery error")
		}

		if !strings.Contains(err.Error(), "casbin: panic removing grouping policy") {
			t.Fatalf("err = %q, want substring %q", err, "casbin: panic removing grouping policy")
		}
	})
}
