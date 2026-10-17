package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeDotEnv(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadEnvBasic(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := writeDotEnv(t, dir, ".env", "DB_ADAPTER=postgres\nDB_MAXCONNS=20\n")
	env, err := LoadEnv(p)
	if err != nil {
		t.Fatal(err)
	}
	if env["DB_ADAPTER"] != "postgres" {
		t.Fatalf("got %q", env["DB_ADAPTER"])
	}
	if env["DB_MAXCONNS"] != "20" {
		t.Fatalf("got %q", env["DB_MAXCONNS"])
	}
}

func TestLoadEnvParserTable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		line  string
		key   string
		value string
	}{
		{name: "bare", line: "A=b", key: "A", value: "b"},
		{name: "spaces around equals", line: "  A  =  b  ", key: "A", value: "b"},
		{name: "empty value", line: "A=", key: "A", value: ""},
		{name: "export prefix", line: "export A=b", key: "A", value: "b"},
		{name: "export padded", line: "  export   A=b", key: "A", value: "b"},
		{name: "single quotes literal", line: `A='a#b $X'`, key: "A", value: "a#b $X"},
		{name: "double quotes", line: `A="a#b"`, key: "A", value: "a#b"},
		{name: "double escapes", line: `A="a\nb\tc\\d\"e"`, key: "A", value: "a\nb\tc\\d\"e"},
		{name: "double dollar escape", line: `A="a\$B"`, key: "A", value: "a$B"},
		{name: "unquoted keeps hash", line: "A=a#b", key: "A", value: "a#b"},
		{name: "unquoted strips spaced comment", line: "A=abc # comment", key: "A", value: "abc"},
		{name: "quoted trailing comment", line: `A="b" # comment`, key: "A", value: "b"},
		{name: "single trailing comment", line: `A='b' # comment`, key: "A", value: "b"},
		{name: "value with equals", line: "A=a=b=c", key: "A", value: "a=b=c"},
		{name: "subshell literal", line: "A=$(whoami)", key: "A", value: "$(whoami)"},
		{name: "backtick literal", line: "A=`whoami`", key: "A", value: "`whoami`"},
		{name: "lowercase key", line: "db_dsn=x", key: "db_dsn", value: "x"},
		{name: "underscore key", line: "_A=b", key: "_A", value: "b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			p := writeDotEnv(t, dir, ".env", tt.line+"\n")
			env, err := LoadEnv(p)
			if err != nil {
				t.Fatal(err)
			}
			if env[tt.key] != tt.value {
				t.Fatalf("got %q, want %q", env[tt.key], tt.value)
			}
		})
	}
}

func TestLoadEnvSkipsBlanksAndComments(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := writeDotEnv(t, dir, ".env", "\n# comment\n   # indented\nA=1\n\n")
	env, err := LoadEnv(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(env) != 1 || env["A"] != "1" {
		t.Fatalf("got %v", env)
	}
}

func TestLoadEnvMalformedFailsClosed(t *testing.T) {
	t.Parallel()
	secret := "s3cr3t-malformed-canary"
	tests := []struct {
		name    string
		content string
	}{
		{name: "no equals", content: "JUSTAKEY\n"},
		{name: "empty key", content: "=value\n"},
		{name: "bad key chars", content: "A-B=c\n"},
		{name: "key starting digit", content: "1A=b\n"},
		{name: "unterminated single", content: "A='abc\n"},
		{name: "unterminated double", content: "A=\"abc\n"},
		{name: "trailing junk after quote", content: "A=\"b\"junk\n"},
		{name: "bad escape", content: "A=\"a\\qb\"\n"},
		{name: "unterminated expansion", content: "A=${B\n"},
		{name: "empty expansion", content: "A=${}\n"},
		{name: "value carries secret", content: "A=" + secret + "\nB\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			p := writeDotEnv(t, dir, ".env", tt.content)
			_, err := LoadEnv(p)
			if err == nil {
				t.Fatal("want error")
			}
			if !strings.Contains(err.Error(), ".env:2") && !strings.Contains(err.Error(), ".env:1") {
				t.Fatalf("error must name file+line: %v", err)
			}
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("error echoes value: %v", err)
			}
		})
	}
}

func TestLoadEnvMissingFileErrors(t *testing.T) {
	t.Parallel()
	_, err := LoadEnv(filepath.Join(t.TempDir(), "nope.env"))
	if err == nil {
		t.Fatal("want error for missing file")
	}
}

func TestLoadEnvLaterFilesWin(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := writeDotEnv(t, dir, "a.env", "K=from-a\nONLY_A=1\n")
	b := writeDotEnv(t, dir, "b.env", "K=from-b\nONLY_B=2\n")
	env, err := LoadEnv(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if env["K"] != "from-b" {
		t.Fatalf("got %q", env["K"])
	}
	if env["ONLY_A"] != "1" || env["ONLY_B"] != "2" {
		t.Fatalf("got %v", env)
	}
}

func TestLoadEnvExpansion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		content string
		key     string
		value   string
	}{
		{name: "basic", content: "B=base\nA=${B}/x\n", key: "A", value: "base/x"},
		{name: "forward ref", content: "A=${B}/x\nB=base\n", key: "A", value: "base/x"},
		{name: "unset empty", content: "A=${NOPE_DEFINED_ANYWHERE}/x\n", key: "A", value: "/x"},
		{name: "single quoted no expand", content: "B=base\nA='${B}'\n", key: "A", value: "${B}"},
		{name: "double quoted expands", content: "B=base\nA=\"${B}\"\n", key: "A", value: "base"},
		{name: "chained", content: "C=c\nB=${C}b\nA=${B}a\n", key: "A", value: "cba"},
		{name: "adjacent", content: "B=1\nC=2\nA=${B}${C}\n", key: "A", value: "12"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			p := writeDotEnv(t, dir, ".env", tt.content)
			env, err := LoadEnv(p)
			if err != nil {
				t.Fatal(err)
			}
			if env[tt.key] != tt.value {
				t.Fatalf("got %q, want %q", env[tt.key], tt.value)
			}
		})
	}
}

func TestLoadEnvExpansionDepthFailsClosed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := writeDotEnv(t, dir, ".env", "A=${A}\n")
	_, err := LoadEnv(p)
	if err == nil {
		t.Fatal("want error for self-referential expansion")
	}
	if strings.Contains(err.Error(), "${A}") {
		t.Fatalf("error echoes value: %v", err)
	}
}

func TestLoadEnvFileSizeBound(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := filepath.Join(dir, ".env")
	big := make([]byte, MaxDotEnvFileSize+1)
	for i := range big {
		big[i] = 'x'
	}
	if err := os.WriteFile(p, big, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadEnv(p)
	if err == nil {
		t.Fatal("want error for oversize file")
	}
}

func TestLoadEnvLineLengthBound(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := writeDotEnv(t, dir, ".env", "A="+strings.Repeat("x", MaxDotEnvLineLength)+"\n")
	_, err := LoadEnv(p)
	if err == nil {
		t.Fatal("want error for overlong line")
	}
	if !strings.Contains(err.Error(), ".env:1") {
		t.Fatalf("error must name file+line: %v", err)
	}
}

func TestFromEnvPrecedence(t *testing.T) {
	dir := t.TempDir()
	writeDotEnv(t, dir, ".env", "K=from-env\nONLY_ENV=1\n")
	writeDotEnv(t, dir, ".env.local", "K=from-local\nONLY_LOCAL=1\n")
	t.Chdir(dir)
	t.Setenv("K", "from-process")
	t.Setenv("ONLY_PROC", "1")

	env, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if env["K"] != "from-process" {
		t.Fatalf("process must win: got %q", env["K"])
	}
	if env["ONLY_ENV"] != "1" || env["ONLY_LOCAL"] != "1" || env["ONLY_PROC"] != "1" {
		t.Fatalf("got %v", env)
	}
}

func TestFromEnvLocalBeatsEnv(t *testing.T) {
	dir := t.TempDir()
	writeDotEnv(t, dir, ".env", "K=from-env\n")
	writeDotEnv(t, dir, ".env.local", "K=from-local\n")
	t.Chdir(dir)

	env, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if env["K"] != "from-local" {
		t.Fatalf("got %q", env["K"])
	}
}

func TestFromEnvNoFiles(t *testing.T) {
	t.Chdir(t.TempDir())
	env, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if env == nil {
		t.Fatal("want non-nil env")
	}
}

func TestFromEnvExpandsAgainstProcess(t *testing.T) {
	dir := t.TempDir()
	writeDotEnv(t, dir, ".env", "A=${FROM_ENV_PROC_BASE}/x\n")
	t.Chdir(dir)
	t.Setenv("FROM_ENV_PROC_BASE", "base")

	env, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if env["A"] != "base/x" {
		t.Fatalf("got %q", env["A"])
	}
}

func TestFromEnvProcessValuesStayLiteral(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("FROM_ENV_LITERAL_TEST", "${NOT_EXPANDED}")

	env, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if env["FROM_ENV_LITERAL_TEST"] != "${NOT_EXPANDED}" {
		t.Fatalf("got %q", env["FROM_ENV_LITERAL_TEST"])
	}
}

func TestWarnLegacyConfigFile(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		file string
	}{
		{name: "yaml", file: "zever.yaml"},
		{name: "yml", file: "zever.yml"},
		{name: "json", file: "zever.json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeDotEnv(t, dir, tt.file, "db:\n  adapter: sqlite\n")
			var sb strings.Builder
			warnLegacyConfigFile(&sb, dir)
			if !strings.Contains(sb.String(), "ignored; see migration guide") {
				t.Fatalf("got %q", sb.String())
			}
			if strings.Count(sb.String(), "\n") != 1 {
				t.Fatalf("want one line, got %q", sb.String())
			}
		})
	}
}

func TestWarnLegacyConfigFileAbsent(t *testing.T) {
	t.Parallel()
	var sb strings.Builder
	warnLegacyConfigFile(&sb, t.TempDir())
	if sb.String() != "" {
		t.Fatalf("got %q", sb.String())
	}
}

func FuzzParseDotEnv(f *testing.F) {
	for _, seed := range []string{
		"A=b\n", "export A='x'\n", "A=\"a\\nb\"\n", "# c\n", "\n",
		"A=$(x)\n", "A=`x`\n", "A=${B}\n", "A=a#b\n", "A=1 # c\n",
		"NOEQUALS\n", "=v\n", "A='unterminated\n",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, content string) {
		t.Parallel()
		dir := t.TempDir()
		p := filepath.Join(dir, ".env")
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		env, err := LoadEnv(p)
		if err != nil {
			if !strings.Contains(err.Error(), "config:") {
				t.Fatalf("error missing prefix: %v", err)
			}
			if !strings.Contains(err.Error(), ".env:") {
				t.Fatalf("error must name file+line: %v", err)
			}
			if len(content) > 20 && strings.Contains(err.Error(), content) {
				t.Fatalf("error echoes input: %v", err)
			}
			return
		}
		for k, v := range env {
			_ = k
			_ = v
		}
	})
}
