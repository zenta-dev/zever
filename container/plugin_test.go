package container

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/zenta-dev/zever/config"
)

var pluginTestSeq atomic.Uint64

func mustUniquePluginName(t *testing.T, prefix string) string {
	t.Helper()
	n := pluginTestSeq.Add(1)
	return prefix + "-" + itoa(n)
}

func itoa(n uint64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

type fakePluginWidget struct {
	label  string
	closed atomic.Int32
}

func (w *fakePluginWidget) Close() error {
	w.closed.Add(1)
	return nil
}

type fakePluginConn struct {
	id     string
	closed atomic.Int32
}

func (c *fakePluginConn) Close(_ context.Context) error {
	c.closed.Add(1)
	return nil
}

func TestPlugin_RegisterAndResolve(t *testing.T) {
	name := mustUniquePluginName(t, "widget")
	if err := RegisterPlugin(name, PluginAPIVersion, func(_ *config.Config) (*fakePluginWidget, error) {
		return &fakePluginWidget{label: "w1"}, nil
	}); err != nil {
		t.Fatalf("RegisterPlugin: %v", err)
	}

	c := New(config.Default())
	v, err := Resolve[*fakePluginWidget](c, name)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if v.label != "w1" {
		t.Fatalf("label = %q, want w1", v.label)
	}
}

func TestPlugin_LazyBuildCount(t *testing.T) {
	name := mustUniquePluginName(t, "lazy")
	var builds atomic.Int32
	if err := RegisterPlugin(name, PluginAPIVersion, func(_ *config.Config) (*fakePluginWidget, error) {
		builds.Add(1)
		return &fakePluginWidget{}, nil
	}); err != nil {
		t.Fatalf("RegisterPlugin: %v", err)
	}

	c := New(config.Default())
	if got := builds.Load(); got != 0 {
		t.Fatalf("builds before Resolve = %d, want 0", got)
	}
	if _, err := Resolve[*fakePluginWidget](c, name); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if _, err := Resolve[*fakePluginWidget](c, name); err != nil {
		t.Fatalf("second Resolve: %v", err)
	}
	if got := builds.Load(); got != 1 {
		t.Fatalf("builds after 2 Resolves = %d, want 1", got)
	}
}

func TestPlugin_DuplicateRegistration(t *testing.T) {
	name := mustUniquePluginName(t, "dup")
	build := func(_ *config.Config) (*fakePluginWidget, error) { return &fakePluginWidget{}, nil }
	if err := RegisterPlugin(name, PluginAPIVersion, build); err != nil {
		t.Fatalf("first RegisterPlugin: %v", err)
	}
	if err := RegisterPlugin(name, PluginAPIVersion, build); err == nil {
		t.Fatal("second RegisterPlugin: got nil, want duplicate error")
	} else if !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("duplicate error = %q, want already registered", err)
	}
}

func TestPlugin_VersionMismatch(t *testing.T) {
	name := mustUniquePluginName(t, "ver")
	err := RegisterPlugin(name, PluginAPIVersion+1, func(_ *config.Config) (*fakePluginWidget, error) {
		return &fakePluginWidget{}, nil
	})
	if err == nil {
		t.Fatal("RegisterPlugin bad version: got nil, want error")
	} else if !strings.Contains(err.Error(), "version mismatch") {
		t.Fatalf("version error = %q, want version mismatch", err)
	}
}

func TestPlugin_EmptyNameAndNilBuild(t *testing.T) {
	if err := RegisterPlugin("", PluginAPIVersion, func(_ *config.Config) (*fakePluginWidget, error) {
		return &fakePluginWidget{}, nil
	}); err == nil {
		t.Error("empty name: got nil, want error")
	}
	if err := RegisterPlugin(mustUniquePluginName(t, "nilbuild"), PluginAPIVersion, (func(*config.Config) (*fakePluginWidget, error))(nil)); err == nil {
		t.Error("nil build: got nil, want error")
	} else if !errors.Is(err, ErrPluginNilBuild) {
		t.Errorf("nil build error = %v, want ErrPluginNilBuild", err)
	}
}

func TestPlugin_UnregisteredName(t *testing.T) {
	c := New(config.Default())
	if _, err := Resolve[*fakePluginWidget](c, "no-such-plugin-zzz"); err == nil {
		t.Fatal("Resolve unregistered: got nil, want error")
	} else if !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("unregistered error = %q, want not registered", err)
	}
}

func TestPlugin_RetryAfterFailure(t *testing.T) {
	name := mustUniquePluginName(t, "retry")
	errBoom := errors.New("boom")
	var calls atomic.Int32
	if err := RegisterPlugin(name, PluginAPIVersion, func(_ *config.Config) (*fakePluginWidget, error) {
		if calls.Add(1) == 1 {
			return nil, errBoom
		}
		return &fakePluginWidget{label: "recovered"}, nil
	}); err != nil {
		t.Fatalf("RegisterPlugin: %v", err)
	}

	c := New(config.Default())
	if _, err := Resolve[*fakePluginWidget](c, name); !errors.Is(err, errBoom) {
		t.Fatalf("first Resolve = %v, want boom", err)
	}
	v, err := Resolve[*fakePluginWidget](c, name)
	if err != nil {
		t.Fatalf("second Resolve: %v", err)
	}
	if v.label != "recovered" {
		t.Fatalf("label = %q, want recovered", v.label)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("build calls = %d, want 2", got)
	}
}

func TestPlugin_TypeMismatch(t *testing.T) {
	name := mustUniquePluginName(t, "mismatch")
	if err := RegisterPlugin(name, PluginAPIVersion, func(_ *config.Config) (*fakePluginWidget, error) {
		return &fakePluginWidget{}, nil
	}); err != nil {
		t.Fatalf("RegisterPlugin: %v", err)
	}

	c := New(config.Default())
	if _, err := Resolve[*fakePluginConn](c, name); err == nil {
		t.Fatal("Resolve wrong type: got nil, want error")
	} else if !strings.Contains(err.Error(), "type mismatch") {
		t.Fatalf("mismatch error = %q, want type mismatch", err)
	}
}

func TestPlugin_CloseParticipates(t *testing.T) {
	name := mustUniquePluginName(t, "closeable")
	w := &fakePluginWidget{label: "c1"}
	if err := RegisterPlugin(name, PluginAPIVersion, func(_ *config.Config) (*fakePluginWidget, error) {
		return w, nil
	}); err != nil {
		t.Fatalf("RegisterPlugin: %v", err)
	}

	c := New(config.Default())
	if _, err := Resolve[*fakePluginWidget](c, name); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if err := c.Close(t.Context()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := w.closed.Load(); got != 1 {
		t.Fatalf("plugin Close calls = %d, want 1", got)
	}
}

func TestPlugin_CloseSkipsUnresolved(t *testing.T) {
	name := mustUniquePluginName(t, "untouched")
	var builds atomic.Int32
	if err := RegisterPlugin(name, PluginAPIVersion, func(_ *config.Config) (*fakePluginWidget, error) {
		builds.Add(1)
		return &fakePluginWidget{}, nil
	}); err != nil {
		t.Fatalf("RegisterPlugin: %v", err)
	}

	c := New(config.Default())
	if err := c.Close(t.Context()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := builds.Load(); got != 0 {
		t.Fatalf("builds after Close without Resolve = %d, want 0", got)
	}
}

func TestPlugin_CloseCtxShape(t *testing.T) {
	name := mustUniquePluginName(t, "ctxclose")
	conn := &fakePluginConn{id: "x"}
	if err := RegisterPlugin(name, PluginAPIVersion, func(_ *config.Config) (*fakePluginConn, error) {
		return conn, nil
	}); err != nil {
		t.Fatalf("RegisterPlugin: %v", err)
	}

	c := New(config.Default())
	if _, err := Resolve[*fakePluginConn](c, name); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if err := c.Close(t.Context()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := conn.closed.Load(); got != 1 {
		t.Fatalf("conn Close(ctx) calls = %d, want 1", got)
	}
}
