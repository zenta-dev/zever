package zevertest_test

import (
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/zevertest"
)

func TestNewTestContainerResolvesDefaults(t *testing.T) {
	t.Parallel()

	c := zevertest.NewTestContainer(t)

	cache, err := c.Cache()
	if err != nil {
		t.Fatalf("Cache: %v", err)
	}

	if setErr := cache.Set(t.Context(), "k", []byte("v"), 0); setErr != nil {
		t.Fatalf("Set: %v", setErr)
	}

	got, err := cache.Get(t.Context(), "k")
	if err != nil || string(got) != "v" {
		t.Fatalf("Get = %q, %v; want v", got, err)
	}

	if _, dbErr := c.DB(); dbErr != nil {
		t.Fatalf("DB: %v", dbErr)
	}

	if _, qErr := c.Queue(); qErr != nil {
		t.Fatalf("Queue: %v", qErr)
	}

	if _, aErr := c.Auth(); aErr != nil {
		t.Fatalf("Auth: %v", aErr)
	}

	if _, rErr := c.Router(); rErr != nil {
		t.Fatalf("Router: %v", rErr)
	}
}

func TestNewTestContainerIsolatesDB(t *testing.T) {
	t.Parallel()

	paths := make(chan string, 2)

	for range 2 {
		c := zevertest.NewTestContainer(t, zevertest.WithConfig(func(cfg *config.Config) {
			paths <- cfg.DB.Options.Path
		}))
		_ = c
	}

	close(paths)

	a, b := <-paths, <-paths
	if a == b || a == "" {
		t.Fatalf("db paths %q and %q must be distinct and non-empty", a, b)
	}
}

func TestWithRegisterRunsBeforeBuild(t *testing.T) {
	t.Parallel()

	ran := false

	zevertest.NewTestContainer(t, zevertest.WithRegister(func() { ran = true }))

	if !ran {
		t.Fatal("WithRegister func did not run")
	}
}

func TestClock(t *testing.T) {
	t.Parallel()

	c := zevertest.NewClock(time.Time{})
	if !c.Now().Equal(zevertest.DefaultStart) {
		t.Fatalf("Now = %v, want %v", c.Now(), zevertest.DefaultStart)
	}

	if got := c.Advance(90 * time.Minute); !got.Equal(zevertest.DefaultStart.Add(90*time.Minute)) || !c.Func()().Equal(got) {
		t.Fatalf("Advance/Func = %v", got)
	}

	want := time.Date(2031, 2, 3, 4, 5, 6, 0, time.UTC)
	c.Set(want)

	if !c.Now().Equal(want) {
		t.Fatalf("Set: Now = %v, want %v", c.Now(), want)
	}

	var wg sync.WaitGroup

	for range 8 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			c.Advance(time.Second)
			_ = c.Now()
		}()
	}

	wg.Wait()

	if !c.Now().Equal(want.Add(8 * time.Second)) {
		t.Fatalf("concurrent Advance lost updates: %v", c.Now())
	}
}

func TestIDs(t *testing.T) {
	t.Parallel()

	g := zevertest.NewIDs("user")
	if a, b := g.Next(), g.Func()(); a != "user-000001" || b != "user-000002" {
		t.Fatalf("Next = %q, %q", a, b)
	}

	if got := zevertest.NewIDs("").Next(); got != "id-000001" {
		t.Fatalf("default prefix: %q", got)
	}
}

func TestClientAgainstContainerRouter(t *testing.T) {
	t.Parallel()

	c := zevertest.NewTestContainer(t)

	r, err := c.Router()
	if err != nil {
		t.Fatalf("Router: %v", err)
	}

	r.Handle("POST", "/echo", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("X-Auth", req.Header.Get("Authorization"))
		w.Header().Set("X-Type", req.Header.Get("Content-Type"))
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	api := zevertest.NewClient(t, r)
	authed := api.WithBearer("tok")

	res := authed.Post("/echo", map[string]string{"a": "b"}).RequireStatus(http.StatusCreated)

	var out struct {
		OK bool `json:"ok"`
	}

	res.JSON(&out)

	if !out.OK || res.Text() != `{"ok":true}` {
		t.Fatalf("body = %q", res.Text())
	}

	if res.Header.Get("X-Auth") != "Bearer tok" || res.Header.Get("X-Type") != "application/json" {
		t.Fatalf("headers = %v", res.Header)
	}

	// The base client is unchanged by WithBearer.
	if got := api.Post("/echo", "raw").Header.Get("X-Auth"); got != "" {
		t.Fatalf("base client leaked bearer: %q", got)
	}

	api.Get("/missing").RequireStatus(http.StatusNotFound)
}
