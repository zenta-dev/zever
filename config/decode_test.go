package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/crypto"
	"github.com/zenta-dev/zever/db"
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
		"DSN":             "postgres://u@h/d",
		"MaxConns":        10,
		"MinConns":        2,
		"MaxConnLifetime": 5_000_000_000,
		"Path":            "/tmp/a.db",
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
	got, err := decodeOptions[db.Options]("db", map[string]any{"maxconns": 7})
	if err != nil {
		t.Fatalf("decodeOptions() error = %v", err)
	}
	if got.MaxConns != 7 {
		t.Errorf("decodeOptions() MaxConns = %d, want 7 (case-insensitive match)", got.MaxConns)
	}
}

func TestDecodeOptions_underscoreKeyRejected(t *testing.T) {
	t.Parallel()
	// encoding/json folds case but treats underscores as significant, so
	// snake_case keys never match untagged Go fields like MaxConns.
	_, err := decodeOptions[db.Options]("db", map[string]any{"max_conns": 7})
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
	path := writeTempConfig(t, "zever.yaml", "db:\n  adapter: sqlite\n  options:\n    MaxConns: 10\n")
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
