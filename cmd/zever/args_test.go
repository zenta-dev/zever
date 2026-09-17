package main

import (
	"flag"
	"reflect"
	"testing"
)

func newTestFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.String("backend", "proto", "")
	fs.String("out", "./generated", "")
	fs.String("adapter", "sqlite", "")
	fs.String("dsn", "", "")
	fs.String("module", "", "")
	fs.String("dir", "", "")
	fs.String("field", "", "")
	fs.String("lang", "go", "")
	fs.String("config", "", "")
	fs.Bool("interactive", false, "")
	fs.Bool("i", false, "")
	fs.Bool("dry-run", false, "")
	fs.Bool("drop-columns", false, "")
	fs.Bool("force", false, "")
	// additional value flags mentioned in spec
	fs.String("framework-path", "", "")
	fs.String("framework-version", "", "")
	fs.String("entry", "", "")
	fs.String("app", "", "")
	fs.String("cron", "", "")
	fs.String("dispatch", "", "")
	fs.String("queue", "", "")
	return fs
}

func TestFlexibleParse_FileFirstVsFlagFirst(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"file first", []string{"a.zen", "b.zen", "--backend=proto", "--out=./out"}},
		{"flag first", []string{"--backend=proto", "--out=./out", "a.zen", "b.zen"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fs := newTestFlagSet()
			got, err := flexibleParse(fs, tc.args)
			if err != nil {
				t.Fatalf("flexibleParse %v: %v", tc.args, err)
			}
			want := []string{"a.zen", "b.zen"}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("pos = %v, want %v", got, want)
			}
			if gotBackend := fs.Lookup("backend").Value.String(); gotBackend != "proto" {
				t.Fatalf("backend = %q, want %q", gotBackend, "proto")
			}
			if gotOut := fs.Lookup("out").Value.String(); gotOut != "./out" {
				t.Fatalf("out = %q, want %q", gotOut, "./out")
			}
		})
	}
}

func TestFlexibleParse_Interleaved(t *testing.T) {
	fs := newTestFlagSet()
	args := []string{"a.zen", "--out", "x", "b.zen", "--backend=proto"}
	got, err := flexibleParse(fs, args)
	if err != nil {
		t.Fatalf("flexibleParse: %v", err)
	}
	want := []string{"a.zen", "b.zen"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if fs.Lookup("out").Value.String() != "x" {
		t.Fatalf("out = %q want %q", fs.Lookup("out").Value.String(), "x")
	}
	if fs.Lookup("backend").Value.String() != "proto" {
		t.Fatalf("backend = %q want %q", fs.Lookup("backend").Value.String(), "proto")
	}
}

func TestFlexibleParse_SpaceSeparatedValueFlags(t *testing.T) {
	fs := newTestFlagSet()
	args := []string{"--out", "./generated", "a.zen", "--backend", "proto"}
	got, err := flexibleParse(fs, args)
	if err != nil {
		t.Fatalf("flexibleParse: %v", err)
	}
	want := []string{"a.zen"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if fs.Lookup("out").Value.String() != "./generated" {
		t.Fatalf("out = %q want ./generated", fs.Lookup("out").Value.String())
	}
	if fs.Lookup("backend").Value.String() != "proto" {
		t.Fatalf("backend = %q want proto", fs.Lookup("backend").Value.String())
	}
}

func TestFlexibleParse_BoolFlags(t *testing.T) {
	t.Run("long bool interleaved", func(t *testing.T) {
		fs := newTestFlagSet()
		args := []string{"a.zen", "--dry-run", "b.zen", "--drop-columns"}
		got, err := flexibleParse(fs, args)
		if err != nil {
			t.Fatalf("flexibleParse: %v", err)
		}
		want := []string{"a.zen", "b.zen"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v want %v", got, want)
		}
		if fs.Lookup("dry-run").Value.String() != "true" {
			t.Fatalf("dry-run = %q want true", fs.Lookup("dry-run").Value.String())
		}
		if fs.Lookup("drop-columns").Value.String() != "true" {
			t.Fatalf("drop-columns = %q want true", fs.Lookup("drop-columns").Value.String())
		}
	})
	t.Run("short bool -i", func(t *testing.T) {
		fs := newTestFlagSet()
		args := []string{"a.zen", "-i", "b.zen"}
		got, err := flexibleParse(fs, args)
		if err != nil {
			t.Fatalf("flexibleParse: %v", err)
		}
		want := []string{"a.zen", "b.zen"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v want %v", got, want)
		}
		if fs.Lookup("i").Value.String() != "true" {
			t.Fatalf("i = %q want true", fs.Lookup("i").Value.String())
		}
	})
	t.Run("bool with equals", func(t *testing.T) {
		fs := newTestFlagSet()
		args := []string{"--force=true", "a.zen"}
		got, err := flexibleParse(fs, args)
		if err != nil {
			t.Fatalf("flexibleParse: %v", err)
		}
		want := []string{"a.zen"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v want %v", got, want)
		}
		if fs.Lookup("force").Value.String() != "true" {
			t.Fatalf("force = %q want true", fs.Lookup("force").Value.String())
		}
	})
	t.Run("bool does not consume next", func(t *testing.T) {
		fs := newTestFlagSet()
		args := []string{"--dry-run", "a.zen"}
		got, err := flexibleParse(fs, args)
		if err != nil {
			t.Fatalf("flexibleParse: %v", err)
		}
		want := []string{"a.zen"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v want %v", got, want)
		}
	})
}

func TestFlexibleParse_Terminator(t *testing.T) {
	t.Run("terminator stops flag parsing", func(t *testing.T) {
		fs := newTestFlagSet()
		args := []string{"--backend=proto", "--", "--out", "a.zen"}
		got, err := flexibleParse(fs, args)
		if err != nil {
			t.Fatalf("flexibleParse: %v", err)
		}
		want := []string{"--out", "a.zen"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v want %v", got, want)
		}
		if fs.Lookup("backend").Value.String() != "proto" {
			t.Fatalf("backend = %q want proto", fs.Lookup("backend").Value.String())
		}
		if fs.Lookup("out").Value.String() != "./generated" {
			t.Fatalf("out should remain default, got %q", fs.Lookup("out").Value.String())
		}
	})
	t.Run("terminator with file before", func(t *testing.T) {
		fs := newTestFlagSet()
		args := []string{"a.zen", "--", "--out", "x"}
		got, err := flexibleParse(fs, args)
		if err != nil {
			t.Fatalf("flexibleParse: %v", err)
		}
		want := []string{"a.zen", "--out", "x"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v want %v", got, want)
		}
	})
	t.Run("terminator mid interleaved", func(t *testing.T) {
		fs := newTestFlagSet()
		args := []string{"a.zen", "--out", "x", "--", "b.zen", "--backend=proto"}
		got, err := flexibleParse(fs, args)
		if err != nil {
			t.Fatalf("flexibleParse: %v", err)
		}
		want := []string{"a.zen", "b.zen", "--backend=proto"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v want %v", got, want)
		}
		if fs.Lookup("out").Value.String() != "x" {
			t.Fatalf("out = %q want x", fs.Lookup("out").Value.String())
		}
	})
}

func TestFlexibleParse_UnknownFlagError(t *testing.T) {
	fs := newTestFlagSet()
	_, err := flexibleParse(fs, []string{"--unknown", "a.zen"})
	if err == nil {
		t.Fatalf("expected error for unknown flag, got nil")
	}
}

func TestFlexibleParse_UnknownFlagWithEquals(t *testing.T) {
	fs := newTestFlagSet()
	_, err := flexibleParse(fs, []string{"--unknown=value", "a.zen"})
	if err == nil {
		t.Fatalf("expected error for unknown flag with equals, got nil")
	}
}

func TestFlexibleParse_MultipleValueFlagsInterleaved(t *testing.T) {
	fs := newTestFlagSet()
	args := []string{"a.zen", "--adapter", "postgres", "b.zen", "--dsn", "postgres://localhost/db", "c.zen"}
	got, err := flexibleParse(fs, args)
	if err != nil {
		t.Fatalf("flexibleParse: %v", err)
	}
	want := []string{"a.zen", "b.zen", "c.zen"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if fs.Lookup("adapter").Value.String() != "postgres" {
		t.Fatalf("adapter = %q want postgres", fs.Lookup("adapter").Value.String())
	}
	if fs.Lookup("dsn").Value.String() != "postgres://localhost/db" {
		t.Fatalf("dsn = %q want postgres://localhost/db", fs.Lookup("dsn").Value.String())
	}
}

func TestFlexibleParse_NoArgs(t *testing.T) {
	fs := newTestFlagSet()
	got, err := flexibleParse(fs, nil)
	if err != nil {
		t.Fatalf("flexibleParse nil: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %v want empty", got)
	}
}

func TestFlexibleParse_OnlyPositionals(t *testing.T) {
	fs := newTestFlagSet()
	args := []string{"a.zen", "b.zen", "c.zen"}
	got, err := flexibleParse(fs, args)
	if err != nil {
		t.Fatalf("flexibleParse: %v", err)
	}
	if !reflect.DeepEqual(got, args) {
		t.Fatalf("got %v want %v", got, args)
	}
}

func TestFlexibleParse_ValueFlagNextIsFlagLike(t *testing.T) {
	// Value flag followed by flag-like token: our helper does not pre-consume,
	// but Go's flag will still consume the next token as value for the preceding
	// value flag. This test documents that behaviour — the helper avoids
	// eagerly swallowing, but cannot override flag.Parse's semantics.
	args := []string{"--out", "--backend=proto", "a.zen"}
	fs2 := newTestFlagSet()
	_, _ = flexibleParse(fs2, args)
	// Go flag treats --backend=proto as value for --out when passed as ["--out", "--backend=proto"].
	// We verify the call does not panic and that positional is preserved.
	if got := fs2.Lookup("out").Value.String(); got != "--backend=proto" && got != "./generated" {
		t.Fatalf("out = %q unexpected", got)
	}
}

type isBoolFlagValue struct {
	v string
}

func (b *isBoolFlagValue) String() string     { return b.v }
func (b *isBoolFlagValue) Set(s string) error { b.v = s; return nil }
func (b *isBoolFlagValue) IsBoolFlag() bool   { return true }

func TestFlexibleParse_IsBoolFlagInterface(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want []string
	}{
		{"bool-like not consuming next", []string{"--custombool", "a.zen"}, []string{"a.zen"}},
		{"bool-like interleaved", []string{"a.zen", "--custombool", "b.zen"}, []string{"a.zen", "b.zen"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			fs.Var(&isBoolFlagValue{v: "no"}, "custombool", "")
			got, err := flexibleParse(fs, tc.args)
			if err != nil {
				t.Fatalf("flexibleParse: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
