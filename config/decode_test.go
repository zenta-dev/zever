package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/analytics"
	"github.com/zenta-dev/zever/billing"
	"github.com/zenta-dev/zever/crypto"
	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/document"
	"github.com/zenta-dev/zever/eventbus"
	"github.com/zenta-dev/zever/flag"
	"github.com/zenta-dev/zever/i18n"
	zredis "github.com/zenta-dev/zever/internal/redis"
	"github.com/zenta-dev/zever/mailer"
	"github.com/zenta-dev/zever/media"
	"github.com/zenta-dev/zever/notification"
	"github.com/zenta-dev/zever/observability"
	"github.com/zenta-dev/zever/payment"
	"github.com/zenta-dev/zever/permission"
	"github.com/zenta-dev/zever/ratelimit"
	"github.com/zenta-dev/zever/scheduler"
	"github.com/zenta-dev/zever/search"
	"github.com/zenta-dev/zever/storage"
	"github.com/zenta-dev/zever/tenant"
	"github.com/zenta-dev/zever/vectorstore"
	"github.com/zenta-dev/zever/webhook"
)

func writeTempConfig(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

func TestDecodeFile(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		file    string
		content string
		want    map[string]ServiceConfig
		wantErr bool
		errIs   error
		errAs   any
	}{
		{
			name: "yaml happy multi-service with nested options",
			file: "zever.yaml",
			content: "db:\n" +
				"  adapter: sqlite\n" +
				"  options:\n" +
				"    Path: /tmp/app.db\n" +
				"    MaxConns: 10\n" +
				"    nested:\n" +
				"      keep: true\n" +
				"crypto:\n" +
				"  adapter: local\n" +
				"  options:\n" +
				"    key: a2V5\n",
			want: map[string]ServiceConfig{
				"db": {
					Adapter: "sqlite",
					Options: map[string]any{
						"Path":     "/tmp/app.db",
						"MaxConns": 10,
						"nested":   map[string]any{"keep": true},
					},
				},
				"crypto": {
					Adapter: "local",
					Options: map[string]any{"key": "a2V5"},
				},
			},
		},
		{
			name: "yml extension decoded as yaml",
			file: "zever.yml",
			content: "db:\n" +
				"  adapter: postgres\n",
			want: map[string]ServiceConfig{
				"db": {Adapter: "postgres"},
			},
		},
		{
			name: "uppercase extension lowercased",
			file: "zever.YAML",
			content: "db:\n" +
				"  adapter: sqlite\n",
			want: map[string]ServiceConfig{
				"db": {Adapter: "sqlite"},
			},
		},
		{
			name: "json happy",
			file: "zever.json",
			content: `{"db": {"adapter": "sqlite", "options": {"Path": "/tmp/a.db", "MaxConns": 4}},` +
				` "crypto": {"adapter": "local"}}`,
			want: map[string]ServiceConfig{
				"db": {
					Adapter: "sqlite",
					Options: map[string]any{"Path": "/tmp/a.db", "MaxConns": float64(4)},
				},
				"crypto": {Adapter: "local"},
			},
		},
		{
			name: "unknown top-level service passes through",
			file: "zever.yaml",
			content: "frobnicate:\n" +
				"  adapter: magic\n" +
				"  options:\n" +
				"    level: 9\n",
			want: map[string]ServiceConfig{
				"frobnicate": {
					Adapter: "magic",
					Options: map[string]any{"level": 9},
				},
			},
		},
		{
			name:    "null service block yields zero envelope",
			file:    "zever.yaml",
			content: "db:\n",
			want: map[string]ServiceConfig{
				"db": {},
			},
		},
		{
			name:    "empty file yields empty map",
			file:    "zever.yaml",
			content: "",
			want:    map[string]ServiceConfig{},
		},
		{
			name:    "unknown key inside service block rejected",
			file:    "zever.yaml",
			content: "db:\n  adaptre: sqlite\n",
			wantErr: true,
			errAs:   &DecodeError{},
		},
		{
			name:    "scalar service entry rejected",
			file:    "zever.json",
			content: `{"db": "sqlite"}`,
			wantErr: true,
			errAs:   &DecodeError{},
		},
		{
			name:    "non-string adapter rejected",
			file:    "zever.json",
			content: `{"db": {"adapter": 7}}`,
			wantErr: true,
			errAs:   &DecodeError{},
		},
		{
			name:    "top-level array rejected",
			file:    "zever.json",
			content: `[{"adapter": "sqlite"}]`,
			wantErr: true,
			errIs:   ErrDecode,
		},
		{
			name:    "top-level list rejected in yaml",
			file:    "zever.yaml",
			content: "- adapter: sqlite\n",
			wantErr: true,
			errIs:   ErrDecode,
		},
		{
			name:    "bad json syntax",
			file:    "zever.json",
			content: `{"db": {"adapter": }}`,
			wantErr: true,
			errIs:   ErrDecode,
		},
		{
			name:    "bad yaml syntax",
			file:    "zever.yaml",
			content: "db:\n\tadapter: [unclosed\n",
			wantErr: true,
			errIs:   ErrDecode,
		},
		{
			name:    "toml unsupported",
			file:    "zever.toml",
			content: "[db]\nadapter = \"sqlite\"\n",
			wantErr: true,
			errIs:   ErrUnsupportedFormat,
		},
		{
			name:    "ini unsupported",
			file:    "zever.ini",
			content: "[db]\nadapter=sqlite\n",
			wantErr: true,
			errIs:   ErrUnsupportedFormat,
		},
		{
			name:    "extensionless unsupported",
			file:    "zever",
			content: `{"db": {}}`,
			wantErr: true,
			errIs:   ErrUnsupportedFormat,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := writeTempConfig(t, tt.file, tt.content)
			got, err := decodeFile(path)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("decodeFile() = %+v, want error", got)
				}
				if tt.errIs != nil && !errors.Is(err, tt.errIs) {
					t.Errorf("decodeFile() error = %v, want errors.Is %v", err, tt.errIs)
				}
				if tt.errAs != nil && !errors.As(err, &tt.errAs) {
					t.Errorf("decodeFile() error = %T, want %T", err, tt.errAs)
				}
				if tt.errIs != nil && errors.Is(err, ErrDecode) && !strings.Contains(err.Error(), filepath.Base(path)) {
					t.Errorf("decodeFile() error = %v, want file path context", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("decodeFile() error = %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("decodeFile() = %+v, want %+v", got, tt.want)
			}
			for name, wantSC := range tt.want {
				gotSC, ok := got[name]
				if !ok {
					t.Errorf("decodeFile() missing service %q", name)
					continue
				}
				if gotSC.Adapter != wantSC.Adapter {
					t.Errorf("service %q adapter = %q, want %q", name, gotSC.Adapter, wantSC.Adapter)
				}
				if len(gotSC.Options) != len(wantSC.Options) {
					t.Errorf("service %q options = %+v, want %+v", name, gotSC.Options, wantSC.Options)
				}
			}
		})
	}
}

func TestDecodeFile_missingFileMentionsPath(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "nope.yaml")
	_, err := decodeFile(path)
	if err == nil {
		t.Fatal("decodeFile() = nil, want error")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("decodeFile() error = %v, want path %q", err, path)
	}
}

func TestDecodeServiceEntry_marshalFailure(t *testing.T) {
	t.Parallel()
	_, err := decodeServiceEntry("db", map[string]any{"f": func() {}})
	if err == nil {
		t.Fatal("decodeServiceEntry() = nil, want error")
	}
	var derr *DecodeError
	if !errors.As(err, &derr) {
		t.Fatalf("decodeServiceEntry() error = %T, want *DecodeError", err)
	}
	if derr.Service != "db" {
		t.Errorf("DecodeError.Service = %q, want %q", derr.Service, "db")
	}
}

func TestDecodeOptions_dbHappy(t *testing.T) {
	t.Parallel()
	got, err := decodeOptions[db.Options]("db", map[string]any{
		"dsn":               "postgres://u@h/d",
		"max_conns":         10,
		"min_conns":         2,
		"max_conn_lifetime": 5_000_000_000,
		"path":              "/tmp/a.db",
	})
	if err != nil {
		t.Fatalf("decodeOptions() error = %v", err)
	}
	if got.DSN != "postgres://u@h/d" || got.MaxConns != 10 || got.MinConns != 2 {
		t.Errorf("decodeOptions() = %+v, want DSN/MaxConns/MinConns set", got)
	}
	if got.MaxConnLifetime != 5_000_000_000 {
		t.Errorf("decodeOptions() MaxConnLifetime = %v, want 5s", got.MaxConnLifetime)
	}
}

func TestDecodeOptions_caseInsensitiveMatch(t *testing.T) {
	t.Parallel()
	got, err := decodeOptions[db.Options]("db", map[string]any{"max_conns": 7})
	if err != nil {
		t.Fatalf("decodeOptions() error = %v", err)
	}
	if got.MaxConns != 7 {
		t.Errorf("decodeOptions() MaxConns = %d, want 7 (case-insensitive match)", got.MaxConns)
	}
}

func TestDecodeOptions_oldFlatKeyRejected(t *testing.T) {
	t.Parallel()
	// Tags are snake_case and strict: the pre-rename flat key "maxconns"
	// no longer matches "max_conns".
	_, err := decodeOptions[db.Options]("db", map[string]any{"maxconns": 7})
	if err == nil {
		t.Fatal("decodeOptions() = nil, want unknown-field error")
	}
	if !errors.Is(err, ErrUnknownField) {
		t.Errorf("decodeOptions() error = %v, want errors.Is ErrUnknownField", err)
	}
}

func TestDecodeOptions_cryptoHappy(t *testing.T) {
	t.Parallel()
	got, err := decodeOptions[crypto.Options]("crypto", map[string]any{"key": "a2V5"})
	if err != nil {
		t.Fatalf("decodeOptions() error = %v", err)
	}
	if got.Key != "a2V5" {
		t.Errorf("decodeOptions() Key = %q, want %q", got.Key, "a2V5")
	}
}

func TestDecodeOptions_unknownField(t *testing.T) {
	t.Parallel()
	_, err := decodeOptions[db.Options]("db", map[string]any{"Bogus": 1})
	if err == nil {
		t.Fatal("decodeOptions() = nil, want error")
	}
	var derr *DecodeError
	if !errors.As(err, &derr) {
		t.Fatalf("decodeOptions() error = %T, want *DecodeError", err)
	}
	if derr.Service != "db" {
		t.Errorf("DecodeError.Service = %q, want %q", derr.Service, "db")
	}
	if !errors.Is(err, ErrUnknownField) {
		t.Errorf("decodeOptions() error = %v, want errors.Is ErrUnknownField", err)
	}
	var uferr *UnknownFieldError
	if !errors.As(err, &uferr) {
		t.Fatalf("decodeOptions() error chain lacks *UnknownFieldError: %v", err)
	}
	if uferr.Service != "db" || uferr.Field != "Bogus" {
		t.Errorf("UnknownFieldError = %+v, want {db Bogus}", uferr)
	}
}

func TestDecodeOptions_typeMismatchHidesValue(t *testing.T) {
	t.Parallel()
	secret := "sup3r-s3cret-value"
	_, err := decodeOptions[db.Options]("db", map[string]any{"MaxConns": secret})
	if err == nil {
		t.Fatal("decodeOptions() = nil, want error")
	}
	var derr *DecodeError
	if !errors.As(err, &derr) {
		t.Fatalf("decodeOptions() error = %T, want *DecodeError", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("decodeOptions() error leaks value: %v", err)
	}

	_, err = decodeOptions[crypto.Options]("crypto", map[string]any{"key": 12345})
	if err == nil {
		t.Fatal("decodeOptions() = nil, want error")
	}
	if strings.Contains(err.Error(), "12345") {
		t.Errorf("decodeOptions() error leaks value: %v", err)
	}
}

func TestDecodeOptions_nonStructTarget(t *testing.T) {
	t.Parallel()
	_, err := decodeOptions[int]("db", map[string]any{"MaxConns": 1})
	if err == nil {
		t.Fatal("decodeOptions() = nil, want error")
	}
	var derr *DecodeError
	if !errors.As(err, &derr) {
		t.Fatalf("decodeOptions() error = %T, want *DecodeError", err)
	}
}

func TestDecodeOptions_emptyAndNil(t *testing.T) {
	t.Parallel()
	got, err := decodeOptions[db.Options]("db", map[string]any{})
	if err != nil {
		t.Fatalf("decodeOptions() error = %v", err)
	}
	if got != (db.Options{}) {
		t.Errorf("decodeOptions() = %+v, want zero", got)
	}
	got, err = decodeOptions[db.Options]("db", nil)
	if err != nil {
		t.Fatalf("decodeOptions() error = %v", err)
	}
	if got != (db.Options{}) {
		t.Errorf("decodeOptions() = %+v, want zero", got)
	}
}

func TestDecodeOptions_durationStringRejected(t *testing.T) {
	t.Parallel()
	// YAML duration strings survive the round-trip as strings, so a
	// time.Duration field rejects them: strict behavior, not silent zero.
	_, err := decodeOptions[db.Options]("db", map[string]any{"MaxConnLifetime": "5s"})
	if err == nil {
		t.Fatal("decodeOptions() = nil, want error")
	}
	var derr *DecodeError
	if !errors.As(err, &derr) {
		t.Fatalf("decodeOptions() error = %T, want *DecodeError", err)
	}
	if strings.Contains(err.Error(), "5s") {
		t.Errorf("decodeOptions() error leaks value: %v", err)
	}
}

func TestDecodeOptions_marshalFailure(t *testing.T) {
	t.Parallel()
	_, err := decodeOptions[db.Options]("db", map[string]any{"DSN": func() {}})
	if err == nil {
		t.Fatal("decodeOptions() = nil, want error")
	}
	var derr *DecodeError
	if !errors.As(err, &derr) {
		t.Fatalf("decodeOptions() error = %T, want *DecodeError", err)
	}
}

type errUnmarshaler struct{}

func (errUnmarshaler) UnmarshalJSON([]byte) error { return errors.New("boom") }

func TestDecodeOptions_unmarshalerFailurePassthrough(t *testing.T) {
	t.Parallel()
	_, err := decodeOptions[errUnmarshaler]("db", map[string]any{"DSN": "x"})
	if err == nil {
		t.Fatal("decodeOptions() = nil, want error")
	}
	var derr *DecodeError
	if !errors.As(err, &derr) {
		t.Fatalf("decodeOptions() error = %T, want *DecodeError", err)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("decodeOptions() error = %v, want underlying cause", err)
	}
}

func TestDecodeOptions_yamlNumbersDecodeAsInts(t *testing.T) {
	t.Parallel()
	path := writeTempConfig(t, "zever.yaml", "db:\n  adapter: sqlite\n  options:\n    max_conns: 10\n")
	raw, err := decodeFile(path)
	if err != nil {
		t.Fatalf("decodeFile() error = %v", err)
	}
	got, err := decodeOptions[db.Options]("db", raw["db"].Options)
	if err != nil {
		t.Fatalf("decodeOptions() error = %v", err)
	}
	if got.MaxConns != 10 {
		t.Errorf("decodeOptions() MaxConns = %d, want 10", got.MaxConns)
	}
}

func TestDecodeOptions_ownedOldFlatKeysRejected(t *testing.T) {
	t.Parallel()
	// Every snake_case rename rejects its pre-rename flat spelling as an
	// unknown field. One representative old key per owned options type.
	tests := []struct {
		name    string
		service string
		decode  func() error
	}{
		{"analytics anonymousid", "analytics", func() error {
			_, err := decodeOptions[analytics.Options]("analytics", map[string]any{"anonymousid": "x"})
			return err
		}},
		{"billing secretkey", "billing", func() error {
			_, err := decodeOptions[billing.Options]("billing", map[string]any{"secretkey": "x"})
			return err
		}},
		{"document tmpdir", "document", func() error {
			_, err := decodeOptions[document.Options]("document", map[string]any{"tmpdir": "/tmp"})
			return err
		}},
		{"eventbus buffersize", "eventbus", func() error {
			_, err := decodeOptions[eventbus.Options]("eventbus", map[string]any{"buffersize": 1})
			return err
		}},
		{"flag firebase projectid", "flag", func() error {
			_, err := decodeOptions[flag.Options]("flag", map[string]any{"firebase": map[string]any{"projectid": "x"}})
			return err
		}},
		{"i18n remote apikey", "i18n", func() error {
			_, err := decodeOptions[i18n.Options]("i18n", map[string]any{"remote": map[string]any{"apikey": "x"}})
			return err
		}},
		{"mailer maxmessagesize", "mailer", func() error {
			_, err := decodeOptions[mailer.Options]("mailer", map[string]any{"maxmessagesize": 1})
			return err
		}},
		{"media maxdownloadbytes", "media", func() error {
			_, err := decodeOptions[media.Options]("media", map[string]any{"maxdownloadbytes": 1})
			return err
		}},
		{"notification twilio accountsid", "notification", func() error {
			_, err := decodeOptions[notification.Options]("notification", map[string]any{"twilio": map[string]any{"accountsid": "x"}})
			return err
		}},
		{"observability servicename", "observability", func() error {
			_, err := decodeOptions[observability.Options]("observability", map[string]any{"servicename": "x"})
			return err
		}},
		{"payment secretkey", "payment", func() error {
			_, err := decodeOptions[payment.Options]("payment", map[string]any{"secretkey": "x"})
			return err
		}},
		{"permission modelpath", "permission", func() error {
			_, err := decodeOptions[permission.Options]("permission", map[string]any{"modelpath": "x"})
			return err
		}},
		{"ratelimit idlettl", "ratelimit", func() error {
			_, err := decodeOptions[ratelimit.Options]("ratelimit", map[string]any{"rate": 1.0, "burst": 1, "idlettl": 1})
			return err
		}},
		{"scheduler closetimeout", "scheduler", func() error {
			_, err := decodeOptions[scheduler.Options]("scheduler", map[string]any{"closetimeout": 1})
			return err
		}},
		{"search apikey", "search", func() error {
			_, err := decodeOptions[search.Options]("search", map[string]any{"apikey": "x"})
			return err
		}},
		{"storage urlbase", "storage", func() error {
			_, err := decodeOptions[storage.Options]("storage", map[string]any{"urlbase": "x"})
			return err
		}},
		{"tenant subdomainregex", "tenant", func() error {
			_, err := decodeOptions[tenant.Options]("tenant", map[string]any{"subdomainregex": "x"})
			return err
		}},
		{"vectorstore apikey", "vectorstore", func() error {
			_, err := decodeOptions[vectorstore.Options]("vectorstore", map[string]any{"apikey": "x"})
			return err
		}},
		{"webhook maxretries", "webhook", func() error {
			_, err := decodeOptions[webhook.Options]("webhook", map[string]any{"maxretries": 1})
			return err
		}},
		{"redis poolsize", "redis", func() error {
			_, err := decodeOptions[zredis.Options]("redis", map[string]any{"poolsize": 1})
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.decode()
			if err == nil {
				t.Fatalf("decodeOptions(%q) = nil, want unknown-field error", tt.service)
			}
			if !errors.Is(err, ErrUnknownField) {
				t.Errorf("decodeOptions(%q) error = %v, want errors.Is ErrUnknownField", tt.service, err)
			}
		})
	}
}
