package container

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	queuedb "github.com/zenta-dev/zever/adapters/queue/db"
	"github.com/zenta-dev/zever/config"
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/shared/ref"
)

// stubOptions is the Options type for the stub service battery below.
type stubOptions struct {
	Label string
}

// stubService is a minimal closable service for provider graph tests. The
// log records Close order; closeErr is returned by Close.
type stubService struct {
	label    string
	mu       *sync.Mutex
	log      *[]string
	closeErr error
}

// newStubLog builds the shared Close-order log for one test.
func newStubLog() (*sync.Mutex, *[]string) {
	return &sync.Mutex{}, &[]string{}
}

// Close records the label and returns the stubbed error.
func (s *stubService) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	*s.log = append(*s.log, s.label)

	return s.closeErr
}

// stubRef names the stub service battery used across provider tests.
func stubRef(name string) ref.Ref[*stubService, stubOptions] {
	return ref.Named[*stubService, stubOptions]("stub", name)
}

// mustCloseContainer registers a cleanup Close on a live context.
// t.Context is canceled before cleanups run, so it cannot bound Close;
// a Background-derived timeout does.
func mustCloseContainer(t *testing.T, c *Container) {
	t.Helper()

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), DefaultCloseTimeout)
		defer cancel()

		if err := c.Close(ctx); err != nil {
			t.Logf("close: %v", err)
		}
	})
}

// mustStub builds a stub factory appending label to log on Close.
func mustStub(label string, mu *sync.Mutex, log *[]string) func(stubOptions) (*stubService, error) {
	return func(o stubOptions) (*stubService, error) {
		return &stubService{label: label + ":" + o.Label, mu: mu, log: log}, nil
	}
}

func TestProvideGetResolvesSingleton(t *testing.T) {
	t.Parallel()

	mu, log := newStubLog()
	c := New(nil, Provide(stubRef("default"),
		mustStub("svc", mu, log),
		WithOptions(func(o *stubOptions) { o.Label = "a" })))
	mustCloseContainer(t, c)

	a, err := Get(c, stubRef("default"))
	if err != nil {
		t.Fatalf("Get = %v, want nil", err)
	}

	b, err := Get(c, stubRef("default"))
	if err != nil {
		t.Fatalf("second Get = %v, want nil", err)
	}

	if a != b {
		t.Fatal("two Gets returned distinct instances, want singleton")
	}

	if a.label != "svc:a" {
		t.Fatalf("label = %q, want WithOptions applied (svc:a)", a.label)
	}
}

func TestProvideDuplicateRef(t *testing.T) {
	t.Parallel()

	mu, log := newStubLog()
	c := New(nil,
		Provide(stubRef("dup"), mustStub("one", mu, log)),
		Provide(stubRef("dup"), mustStub("two", mu, log)))
	mustCloseContainer(t, c)

	_, err := Get(c, stubRef("dup"))
	if err == nil {
		t.Fatal("Get(duplicate) = nil, want DuplicateRefError")
	}

	var dup DuplicateRefError
	if !errors.As(err, &dup) {
		t.Fatalf("Get(duplicate) error type = %T, want DuplicateRefError", err)
	}

	if !errors.Is(err, ErrDuplicateRef) {
		t.Fatalf("Get(duplicate) does not match ErrDuplicateRef: %v", err)
	}

	if dup.Service != "stub" || dup.Name != "dup" {
		t.Fatalf("DuplicateRefError = %+v, want service stub name dup", dup)
	}

	if dup.First == "" || dup.Second == "" || dup.First == dup.Second {
		t.Fatalf("DuplicateRefError sites = %q/%q, want two distinct file:line sites", dup.First, dup.Second)
	}

	if !strings.Contains(err.Error(), "provide_test.go") {
		t.Fatalf("DuplicateRefError %q names no declaration site", err)
	}

	if got := c.Wired(); len(got) != 0 {
		t.Fatalf("Wired with only a duplicate = %v, want empty", got)
	}
}

func TestProvideMissingDep(t *testing.T) {
	t.Parallel()

	mu, log := newStubLog()
	dep := stubRef("missing")
	c := New(nil, Provide1(stubRef("default"), dep,
		func(_ stubOptions, _ *stubService) (*stubService, error) {
			return &stubService{label: "top", mu: mu, log: log}, nil
		}))
	mustCloseContainer(t, c)

	_, err := Get(c, stubRef("default"))
	if err == nil {
		t.Fatal("Get with missing dep = nil, want NotWiredError")
	}

	var nw NotWiredError
	if !errors.As(err, &nw) {
		t.Fatalf("error type = %T, want NotWiredError", err)
	}

	if !errors.Is(err, ErrNotWired) {
		t.Fatalf("error does not match ErrNotWired: %v", err)
	}

	if nw.Service != "stub" || nw.Name != "missing" {
		t.Fatalf("NotWiredError = %+v, want missing stub/missing", nw)
	}

	if nw.Dependent == "" || nw.Site == "" || nw.Fix == "" {
		t.Fatalf("NotWiredError = %+v, want dependent, site, and fix", nw)
	}
}

func TestProvideCycle(t *testing.T) {
	t.Parallel()

	mu, log := newStubLog()
	a := stubRef("a")
	b := stubRef("b")
	c := New(nil,
		Provide1(a, b, func(_ stubOptions, _ *stubService) (*stubService, error) {
			return &stubService{label: "a", mu: mu, log: log}, nil
		}),
		Provide1(b, a, func(_ stubOptions, _ *stubService) (*stubService, error) {
			return &stubService{label: "b", mu: mu, log: log}, nil
		}))
	mustCloseContainer(t, c)

	_, err := Get(c, a)
	if err == nil {
		t.Fatal("Get in cycle = nil, want DependencyCycleError")
	}

	var cyc DependencyCycleError
	if !errors.As(err, &cyc) {
		t.Fatalf("error type = %T, want DependencyCycleError", err)
	}

	if !errors.Is(err, ErrDependencyCycle) {
		t.Fatalf("error does not match ErrDependencyCycle: %v", err)
	}

	if len(cyc.Path) < 3 {
		t.Fatalf("cycle path = %v, want the loop spelled out", cyc.Path)
	}

	if _, err := Get(c, b); err == nil {
		t.Fatal("Get of the other cycle member = nil, want DependencyCycleError")
	}
}

func TestProvideOptionalMissingDepRuns(t *testing.T) {
	t.Parallel()

	mu, log := newStubLog()
	var gotDep *stubService

	c := New(nil, Provide1(stubRef("default"), stubRef("absent"),
		func(_ stubOptions, d *stubService) (*stubService, error) {
			gotDep = d

			return &stubService{label: "top", mu: mu, log: log}, nil
		},
		Optional()))
	mustCloseContainer(t, c)

	if _, err := Get(c, stubRef("default")); err != nil {
		t.Fatalf("Get with absent Optional dep = %v, want nil", err)
	}

	if gotDep != nil {
		t.Fatalf("Optional dep = %+v, want zero value (nil)", gotDep)
	}
}

func TestProvideLazyAndClearOnError(t *testing.T) {
	t.Parallel()

	calls := 0
	c := New(nil, Provide(stubRef("default"),
		func(stubOptions) (*stubService, error) {
			calls++

			if calls == 1 {
				return nil, errors.New("container: stub boom")
			}

			mu, log := newStubLog()

			return &stubService{label: "recovered", mu: mu, log: log}, nil
		}))
	mustCloseContainer(t, c)

	if _, err := Get(c, stubRef("default")); err == nil {
		t.Fatal("first Get = nil, want the factory error")
	}

	v, err := Get(c, stubRef("default"))
	if err != nil {
		t.Fatalf("second Get = %v, want retry to succeed", err)
	}

	if v.label != "recovered" {
		t.Fatalf("label = %q, want recovered", v.label)
	}

	if calls != 2 {
		t.Fatalf("factory calls = %d, want exactly 2 (no caching of failure)", calls)
	}
}

func TestProvideTwoDBsQueueDependsOnOne(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	one := coredb.Ref("one")
	two := coredb.Ref("two")

	var queueSaw coredb.DB

	c := New(nil,
		Provide(one, func(o coredb.Options) (coredb.DB, error) {
			o.Path = filepath.Join(t.TempDir(), "one.db")

			return dbsqlite.New(o)
		}),
		Provide(two, func(o coredb.Options) (coredb.DB, error) {
			o.Path = filepath.Join(t.TempDir(), "two.db")

			return dbsqlite.New(o)
		}),
		Provide1(queue.DefaultRef(), one,
			func(_ queue.Options, d coredb.DB) (queue.Queue, error) {
				queueSaw = d

				return queuedb.NewFromDB(d, queuedb.Options{})
			}))
	mustCloseContainer(t, c)

	q, err := Get(c, queue.DefaultRef())
	if err != nil {
		t.Fatalf("Get(queue) = %v, want nil", err)
	}

	dbOne, err := Get(c, one)
	if err != nil {
		t.Fatalf("Get(db one) = %v, want nil", err)
	}

	dbTwo, err := Get(c, two)
	if err != nil {
		t.Fatalf("Get(db two) = %v, want nil", err)
	}

	if queueSaw != dbOne {
		t.Fatal("queue was not built over exactly db one")
	}

	if dbOne == dbTwo {
		t.Fatal("db one and db two share an instance, want distinct pools")
	}

	if err := q.Push(ctx, "jobs", queue.Payload("hi"), nil); err != nil {
		t.Fatalf("Push = %v, want nil", err)
	}

	msg, popErr := q.Pop(ctx, "jobs")
	if popErr != nil {
		t.Fatalf("Pop = %v, want nil", popErr)
	}

	if string(msg.Payload) != "hi" {
		t.Fatalf("Pop payload = %q, want hi", msg.Payload)
	}
}

func TestProvideEagerBuildsAtNew(t *testing.T) {
	t.Parallel()

	built := false
	c := New(nil, Provide(stubRef("default"),
		func(stubOptions) (*stubService, error) {
			built = true
			mu, log := newStubLog()

			return &stubService{label: "eager", mu: mu, log: log}, nil
		},
		Eager()))
	mustCloseContainer(t, c)

	if !built {
		t.Fatal("Eager provider was not built during New")
	}

	if _, err := Get(c, stubRef("default")); err != nil {
		t.Fatalf("Get(eager) = %v, want nil", err)
	}
}

func TestProvideEagerFailureIsolated(t *testing.T) {
	t.Parallel()

	bad := stubRef("bad")
	good := stubRef("good")
	mu, log := newStubLog()

	c := New(nil,
		Provide(bad, func(stubOptions) (*stubService, error) {
			return nil, errors.New("container: stub bad boom")
		}, Eager()),
		Provide(good, mustStub("good", mu, log)))
	mustCloseContainer(t, c)

	if _, err := Get(c, bad); err == nil {
		t.Fatal("Get(failed eager) = nil, want the build error")
	}

	if _, err := Get(c, good); err != nil {
		t.Fatalf("Get(unrelated) = %v, want nil", err)
	}
}

func TestProvideCloseOrder(t *testing.T) {
	t.Parallel()

	mu, log := newStubLog()
	dbRef := stubRef("db")
	qRef := stubRef("queue")

	c := New(nil,
		Provide(dbRef, mustStub("db", mu, log)),
		Provide1(qRef, dbRef, func(_ stubOptions, _ *stubService) (*stubService, error) {
			return &stubService{label: "queue:", mu: mu, log: log}, nil
		}))
	mustCloseContainer(t, c)

	if _, err := Get(c, qRef); err != nil {
		t.Fatalf("Get(queue) = %v, want nil", err)
	}

	if _, err := Get(c, dbRef); err != nil {
		t.Fatalf("Get(db) = %v, want nil", err)
	}

	// Resolve through Close only: deterministic reverse-dependency order.
	ctx, cancel := context.WithTimeout(context.Background(), DefaultCloseTimeout)
	defer cancel()

	if err := c.Close(ctx); err != nil {
		t.Fatalf("Close = %v, want nil", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(*log) != 2 || (*log)[0] != "queue:" || (*log)[1] != "db:" {
		t.Fatalf("close order = %v, want [queue: db:]", *log)
	}
}

func TestProvideWired(t *testing.T) {
	t.Parallel()

	mu, log := newStubLog()
	c := New(nil,
		Provide(stubRef("b"), mustStub("b", mu, log)),
		Provide(stubRef("a"), mustStub("a", mu, log)))
	mustCloseContainer(t, c)

	got := c.Wired()
	if len(got) != 2 || got[0] != "stub/a" || got[1] != "stub/b" {
		t.Fatalf("Wired = %v, want [stub/a stub/b]", got)
	}
}

func TestProvideUnwiredGet(t *testing.T) {
	t.Parallel()

	c := New(config.Default())
	mustCloseContainer(t, c)

	if got := c.Wired(); len(got) != 0 {
		t.Fatalf("Wired with zero providers = %v, want empty", got)
	}

	_, err := Get(c, stubRef("nope"))
	if err == nil {
		t.Fatal("Get(unwired) = nil, want NotWiredError")
	}

	var nw NotWiredError
	if !errors.As(err, &nw) {
		t.Fatalf("error type = %T, want NotWiredError", err)
	}
}

func TestProvideOptionalPresentDepResolves(t *testing.T) {
	t.Parallel()

	mu, log := newStubLog()
	dep := stubRef("dep")
	var gotDep *stubService

	c := New(nil,
		Provide(dep, mustStub("dep", mu, log)),
		Provide1(stubRef("default"), dep,
			func(_ stubOptions, d *stubService) (*stubService, error) {
				gotDep = d

				return &stubService{label: "top", mu: mu, log: log}, nil
			},
			Optional()))
	mustCloseContainer(t, c)

	if _, err := Get(c, stubRef("default")); err != nil {
		t.Fatalf("Get = %v, want nil", err)
	}

	if gotDep == nil {
		t.Fatal("present Optional dep resolved to nil, want the wired instance")
	}
}
