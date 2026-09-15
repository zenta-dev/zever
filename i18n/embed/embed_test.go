package embed

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/zenta-dev/zever/i18n"
)

func testFS() fstest.MapFS {
	return fstest.MapFS{
		"en.json": {Data: []byte(`{"hello":"Hello, {{.name}}!","bye":"Goodbye"}`)},
		"fr.json": {Data: []byte(`{"hello":"Bonjour, {{.name}}!"}`)},
	}
}

func mustNew(t *testing.T, fs fstest.MapFS, fallback string) i18n.I18n {
	t.Helper()
	be, err := New(i18n.Options{Embed: i18n.EmbedOptions{FS: fs, Dir: ".", Fallback: fallback}})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = be.Close() })
	return be
}

func TestTranslate_exactHit(t *testing.T) {
	t.Parallel()
	be := mustNew(t, testFS(), "en")
	got, err := be.Translate(context.Background(), "en", "hello", map[string]string{"name": "Ada"})
	if err != nil {
		t.Fatalf("Translate() error = %v", err)
	}
	if got != "Hello, Ada!" {
		t.Errorf("Translate() = %q, want %q", got, "Hello, Ada!")
	}
}

func TestTranslate_baseLocaleStrip(t *testing.T) {
	t.Parallel()
	be := mustNew(t, testFS(), "en")
	got, err := be.Translate(context.Background(), "en-US", "bye", map[string]string{"name": "x"})
	if err != nil {
		t.Fatalf("Translate() error = %v", err)
	}
	if got != "Goodbye" {
		t.Errorf("Translate() = %q, want %q", got, "Goodbye")
	}
}

func TestTranslate_fallbackChain(t *testing.T) {
	t.Parallel()
	be := mustNew(t, testFS(), "en")
	got, err := be.Translate(context.Background(), "fr", "bye", map[string]string{"name": "x"})
	if err != nil {
		t.Fatalf("Translate() error = %v", err)
	}
	if got != "Goodbye" {
		t.Errorf("Translate() = %q, want %q (per-key fallback to en)", got, "Goodbye")
	}
}

func TestTranslate_missingLocale(t *testing.T) {
	t.Parallel()
	be := mustNew(t, testFS(), "")
	_, err := be.Translate(context.Background(), "de", "hello", map[string]string{"name": "x"})
	if !errors.Is(err, i18n.ErrLocaleNotFound) && !errors.Is(err, i18n.ErrKeyNotFound) {
		t.Fatalf("Translate() error = %v, want ErrLocaleNotFound or ErrKeyNotFound", err)
	}
	var knf *i18n.KeyNotFoundError
	var lnf *i18n.LocaleNotFoundError
	if !errors.As(err, &knf) && !errors.As(err, &lnf) {
		t.Fatalf("Translate() error type = %T, want *KeyNotFoundError or *LocaleNotFoundError", err)
	}
	if knf != nil && (knf.Locale != "de" || knf.Key != "hello") {
		t.Errorf("KeyNotFoundError = %+v, want Locale=de Key=hello", knf)
	}
}

func TestTranslate_missingKey(t *testing.T) {
	t.Parallel()
	be := mustNew(t, testFS(), "en")
	_, err := be.Translate(context.Background(), "en", "nope", nil)
	if !errors.Is(err, i18n.ErrKeyNotFound) {
		t.Fatalf("Translate() error = %v, want ErrKeyNotFound", err)
	}
	var knf *i18n.KeyNotFoundError
	if !errors.As(err, &knf) {
		t.Fatalf("Translate() error type = %T, want *KeyNotFoundError", err)
	}
	if knf.Locale != "en" || knf.Key != "nope" {
		t.Errorf("KeyNotFoundError = %+v, want Locale=en Key=nope", knf)
	}
}

func TestTranslate_missingArgExecuteError(t *testing.T) {
	t.Parallel()
	be := mustNew(t, testFS(), "en")
	_, err := be.Translate(context.Background(), "en", "hello", map[string]string{})
	if err == nil {
		t.Fatal("Translate() with empty args on action template = nil error, want missingkey error")
	}
}

func TestTranslate_emptyLocale(t *testing.T) {
	t.Parallel()
	be := mustNew(t, testFS(), "en")
	_, err := be.Translate(context.Background(), "   ", "hello", nil)
	if !errors.Is(err, i18n.ErrLocaleNotFound) {
		t.Fatalf("Translate() error = %v, want ErrLocaleNotFound", err)
	}
	var lnf *i18n.LocaleNotFoundError
	if !errors.As(err, &lnf) {
		t.Fatalf("Translate() error type = %T, want *LocaleNotFoundError", err)
	}
}

func TestNew_badJSON(t *testing.T) {
	t.Parallel()
	fs := fstest.MapFS{"en.json": {Data: []byte(`{broken`)}}
	_, err := New(i18n.Options{Embed: i18n.EmbedOptions{FS: fs, Dir: "."}})
	if err == nil {
		t.Fatal("New() with bad JSON = nil error, want decode error")
	}
}

func TestNew_badTemplateFailFast(t *testing.T) {
	t.Parallel()
	fs := fstest.MapFS{"en.json": {Data: []byte(`{"bad":"{{.unclosed"}`)}}
	_, err := New(i18n.Options{Embed: i18n.EmbedOptions{FS: fs, Dir: "."}})
	if err == nil {
		t.Fatal("New() with bad template = nil error, want fail-fast parse error")
	}
}

func TestNew_badLocaleFilename(t *testing.T) {
	t.Parallel()
	fs := fstest.MapFS{"e n!.json": {Data: []byte(`{"a":"b"}`)}}
	_, err := New(i18n.Options{Embed: i18n.EmbedOptions{FS: fs, Dir: "."}})
	if !errors.Is(err, i18n.ErrInvalidOptions) {
		t.Fatalf("New() error = %v, want ErrInvalidOptions", err)
	}
}

func TestNew_nilFS(t *testing.T) {
	t.Parallel()
	_, err := New(i18n.Options{})
	if !errors.Is(err, i18n.ErrInvalidOptions) {
		t.Fatalf("New() error = %v, want ErrInvalidOptions", err)
	}
}

func TestLocales_sorted(t *testing.T) {
	t.Parallel()
	be := mustNew(t, fstest.MapFS{
		"fr.json": {Data: []byte(`{"a":"b"}`)},
		"en.json": {Data: []byte(`{"a":"b"}`)},
		"de.json": {Data: []byte(`{"a":"b"}`)},
	}, "")
	got, err := be.Locales(context.Background())
	if err != nil {
		t.Fatalf("Locales() error = %v", err)
	}
	want := []string{"de", "en", "fr"}
	if len(got) != len(want) {
		t.Fatalf("Locales() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Locales() = %v, want %v", got, want)
		}
	}
}

func TestAfterClose(t *testing.T) {
	t.Parallel()
	be, err := New(i18n.Options{Embed: i18n.EmbedOptions{FS: testFS(), Dir: "."}})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := be.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := be.Close(); err != nil {
		t.Fatalf("second Close() error = %v, want nil (idempotent)", err)
	}
	if _, err := be.Translate(context.Background(), "en", "hello", nil); !errors.Is(err, i18n.ErrClosed) {
		t.Errorf("Translate() after Close error = %v, want ErrClosed", err)
	}
	if _, err := be.Locales(context.Background()); !errors.Is(err, i18n.ErrClosed) {
		t.Errorf("Locales() after Close error = %v, want ErrClosed", err)
	}
}

func TestConcurrentTranslate(t *testing.T) {
	t.Parallel()
	be := mustNew(t, testFS(), "en")
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 25; j++ {
				if _, err := be.Translate(context.Background(), "en", "hello", map[string]string{"name": "Ada"}); err != nil {
					t.Errorf("Translate() error = %v", err)
					return
				}
				if _, err := be.Locales(context.Background()); err != nil {
					t.Errorf("Locales() error = %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()
}

func TestTranslate_cancelledContext(t *testing.T) {
	t.Parallel()
	be := mustNew(t, testFS(), "en")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := be.Translate(ctx, "en", "hello", nil); err == nil {
		t.Error("Translate() with cancelled ctx = nil error, want ctx.Err()")
	}
	if _, err := be.Locales(ctx); err == nil {
		t.Error("Locales() with cancelled ctx = nil error, want ctx.Err()")
	}
}
