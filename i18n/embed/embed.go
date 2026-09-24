package embed

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"text/template"

	"github.com/zenta-dev/zever/codec"
	"github.com/zenta-dev/zever/i18n"
)

// localeCodec decodes a single locale's JSON catalog file (key -> message
// template string).
var localeCodec = codec.JSONCodec[map[string]string]{}

// driver is an embedded-catalog i18n driver.
type driver struct {
	mu       sync.RWMutex
	catalogs map[string]map[string]*template.Template
	fallback string
	closed   atomic.Bool
}

var _ i18n.I18n = (*driver)(nil)

// New builds an i18n.I18n from JSON catalogs in an fs.FS directory.
func New(opts i18n.Options) (i18n.I18n, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}
	if opts.Embed.FS == nil {
		return nil, &i18n.InvalidOptionsError{Reason: "embed fs is required"}
	}
	dir := opts.Embed.Dir
	if dir == "" {
		dir = "."
	}

	entries, err := fs.ReadDir(opts.Embed.FS, dir)
	if err != nil {
		return nil, fmt.Errorf("embed: read dir: %w", err)
	}

	catalogs := make(map[string]map[string]*template.Template)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		locale := strings.TrimSuffix(entry.Name(), ".json")
		if !validLocaleName(locale) {
			return nil, &i18n.InvalidOptionsError{Reason: fmt.Sprintf("invalid locale file name %q", entry.Name())}
		}
		data, err := fs.ReadFile(opts.Embed.FS, path.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("embed: read %q: %w", entry.Name(), err)
		}
		raw, err := localeCodec.Decode(data)
		if err != nil {
			return nil, fmt.Errorf("embed: decode %q: %w", entry.Name(), err)
		}
		cat := make(map[string]*template.Template, len(raw))
		for key, v := range raw {
			tmpl, err := template.New(key).Option("missingkey=error").Parse(v)
			if err != nil {
				return nil, fmt.Errorf("embed: parse template %q (locale %q): %w", key, locale, err)
			}
			cat[key] = tmpl
		}
		catalogs[locale] = cat
	}

	return &driver{catalogs: catalogs, fallback: opts.Embed.Fallback}, nil
}

// Translate returns the message for locale and key, interpolating args.
// The locale chain (locale, base language, fallback) is probed per key:
// the first catalog containing the key wins.
func (d *driver) Translate(ctx context.Context, locale, key string, args map[string]string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if d.closed.Load() {
		return "", i18n.ErrClosed
	}
	if strings.TrimSpace(locale) == "" {
		return "", &i18n.LocaleNotFoundError{Locale: locale}
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	for _, loc := range i18n.LocaleChain(locale, d.fallback) {
		cat, ok := d.catalogs[loc]
		if !ok {
			continue
		}
		tmpl, ok := cat[key]
		if !ok {
			continue
		}
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, args); err != nil {
			return "", fmt.Errorf("embed: execute template %q (locale %q): %w", key, loc, err)
		}
		return buf.String(), nil
	}
	return "", &i18n.KeyNotFoundError{Locale: locale, Key: key}
}

// Locales lists the available locales in sorted order.
func (d *driver) Locales(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if d.closed.Load() {
		return nil, i18n.ErrClosed
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	out := make([]string, 0, len(d.catalogs))
	for loc := range d.catalogs {
		out = append(out, loc)
	}
	sort.Strings(out)
	return out, nil
}

// Close releases driver resources. It is idempotent.
func (d *driver) Close() error {
	if d.closed.Swap(true) {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.catalogs = nil
	return nil
}

// validLocaleName reports whether s matches ^[A-Za-z]{2,3}(-[A-Za-z0-9]+)*$.
func validLocaleName(s string) bool {
	parts := strings.Split(s, "-")
	if len(parts) == 0 || !isASCIILetters(parts[0]) || len(parts[0]) < 2 || len(parts[0]) > 3 {
		return false
	}
	for _, p := range parts[1:] {
		if p == "" || !isASCIIAlnum(p) {
			return false
		}
	}
	return true
}

func isASCIILetter(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z'
}

func isASCIIDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func isASCIILetters(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isASCIILetter(s[i]) {
			return false
		}
	}
	return true
}

func isASCIIAlnum(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isASCIILetter(s[i]) && !isASCIIDigit(s[i]) {
			return false
		}
	}
	return true
}
