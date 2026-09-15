package embed

import (
	"context"
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	"github.com/zenta-dev/zever/i18n"
)

var errReadFileSentinel = errors.New("sentinel read failure")

type readErrFS struct {
	fstest.MapFS
}

func (readErrFS) ReadFile(string) ([]byte, error) {
	return nil, errReadFileSentinel
}

func TestNew_validateError(t *testing.T) {
	t.Parallel()
	_, err := New(i18n.Options{
		Embed:  i18n.EmbedOptions{FS: testFS(), Dir: "."},
		Remote: i18n.RemoteOptions{Timeout: -time.Second},
	})
	if !errors.Is(err, i18n.ErrInvalidOptions) {
		t.Fatalf("New() error = %v, want ErrInvalidOptions", err)
	}
}

func TestNew_defaultDir(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{"en.json": {Data: []byte(`{"a":"b"}`)}}
	be, err := New(i18n.Options{Embed: i18n.EmbedOptions{FS: fsys}})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = be.Close() })
	got, err := be.Locales(context.Background())
	if err != nil {
		t.Fatalf("Locales() error = %v", err)
	}
	if len(got) != 1 || got[0] != "en" {
		t.Fatalf("Locales() = %v, want [en]", got)
	}
}

func TestNew_readDirError(t *testing.T) {
	t.Parallel()
	_, err := New(i18n.Options{Embed: i18n.EmbedOptions{FS: testFS(), Dir: "missing"}})
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("New() error = %v, want wrapped ErrNotExist", err)
	}
}

func TestNew_skipNonJSON(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		"readme.txt": {Data: []byte(`hi`)},
		"sub":        {Mode: fs.ModeDir},
		"sub/x.json": {Data: []byte(`{"a":"b"}`)},
		"en.json":    {Data: []byte(`{"a":"b"}`)},
	}
	be, err := New(i18n.Options{Embed: i18n.EmbedOptions{FS: fsys, Dir: "."}})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = be.Close() })
	got, err := be.Locales(context.Background())
	if err != nil {
		t.Fatalf("Locales() error = %v", err)
	}
	if len(got) != 1 || got[0] != "en" {
		t.Fatalf("Locales() = %v, want [en]", got)
	}
}

func TestNew_readFileError(t *testing.T) {
	t.Parallel()
	fsys := readErrFS{fstest.MapFS{"en.json": {Data: []byte(`{"a":"b"}`)}}}
	_, err := New(i18n.Options{Embed: i18n.EmbedOptions{FS: fsys, Dir: "."}})
	if !errors.Is(err, errReadFileSentinel) {
		t.Fatalf("New() error = %v, want wrapped %v", err, errReadFileSentinel)
	}
}

func TestValidLocaleName(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"empty", "", false},
		{"singleLetter", "e", false},
		{"fourLetters", "abcd", false},
		{"nonASCII", "é", false},
		{"twoLetters", "ab", true},
		{"basic", "en", true},
		{"complex", "zh-Hant", true},
		{"digitPart", "en-1", true},
		{"badCharPart", "en-!", false},
		{"trailingDash", "en-", false},
		{"emptyMiddlePart", "en--x", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := validLocaleName(tc.in); got != tc.want {
				t.Errorf("validLocaleName(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestIsASCIILetter(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   byte
		want bool
	}{
		{"lower", 'a', true},
		{"upper", 'Z', true},
		{"digit", '0', false},
		{"symbol", '!', false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := isASCIILetter(tc.in); got != tc.want {
				t.Errorf("isASCIILetter(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestIsASCIIDigit(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   byte
		want bool
	}{
		{"zero", '0', true},
		{"nine", '9', true},
		{"letter", 'a', false},
		{"symbol", '!', false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := isASCIIDigit(tc.in); got != tc.want {
				t.Errorf("isASCIIDigit(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestIsASCIILetters(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"empty", "", false},
		{"letters", "abc", true},
		{"mixed", "a1", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := isASCIILetters(tc.in); got != tc.want {
				t.Errorf("isASCIILetters(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestIsASCIIAlnum(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"empty", "", false},
		{"alnum", "a1", true},
		{"digitOnly", "1", true},
		{"symbol", "a!", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := isASCIIAlnum(tc.in); got != tc.want {
				t.Errorf("isASCIIAlnum(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
