package config

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// MaxDotEnvFileSize bounds a single .env file at 10 MiB. Larger files fail
// closed instead of being read into memory.
const MaxDotEnvFileSize = 10 * 1024 * 1024

// MaxDotEnvLineLength bounds a single .env line at 1 MiB. Longer lines fail
// closed with the file and line number.
const MaxDotEnvLineLength = 1 * 1024 * 1024

// DotEnvMaxExpansionDepth bounds ${VAR} expansion passes over merged .env
// values. Self-referential values stop converging and fail closed.
const DotEnvMaxExpansionDepth = 16

// dotEnvEntry is one parsed KEY=VALUE line with its provenance.
type dotEnvEntry struct {
	key    string
	value  string
	expand bool
	origin string
}

// LoadEnv reads the listed dotenv files in order (later files win) and
// returns the merged map. Files use KEY=VALUE lines with single/double
// quotes, `#` comments, and an optional `export` prefix; values may
// reference earlier keys via ${VAR} (unset names expand to empty).
//
// LoadEnv parses dotenv files ONLY: the KEY=VALUE format above, never
// yaml/json/toml. There is no config-file format behind it — a yaml file
// is not a dotenv file, and a `KEY: VALUE` line is a malformed line, not a
// mapping.
//
// LoadEnv is the explicit-file half of the pair: the caller names the exact
// files to read, and LoadEnv returns their parsed map without touching the
// process environment. FromEnv is the discovery half: it finds `.env` and
// `.env.local` in the working directory and overlays the process environment
// on top. Both feed the same Env layer; see Resolve.
//
// LoadEnv is strict about presence: a missing file is an error. It is
// fail-closed about content: a malformed line fails with the file and line
// number, never the value. `$(...)` and backticks are literal text, never
// executed. Expansion uses file values only; FromEnv additionally lets
// values reference the process environment.
func LoadEnv(files ...string) (Env, error) {
	entries, err := readDotEnvFiles(files)
	if err != nil {
		return nil, err
	}
	env, expandable, origins := mergeDotEnvEntries(entries)
	if err := expandDotEnv(env, expandable, origins); err != nil {
		return nil, err
	}
	return env, nil
}

// FromEnv builds the process Env from `.env` and `.env.local` in the
// working directory (`.env.local` wins) overlaid by the real process
// environment (process wins). Missing files are skipped silently; present
// files parse under the same fail-closed rules as LoadEnv, and their values
// may reference process variables via ${VAR}. Process values stay literal
// and are never re-expanded.
//
// FromEnv reads dotenv files only: no other file in the working directory
// is consulted, so a leftover config file from an older release is inert
// and is not reported.
func FromEnv() (Env, error) {
	var files []string
	for _, name := range []string{".env", ".env.local"} {
		if info, err := os.Stat(name); err == nil && !info.IsDir() {
			files = append(files, name)
		}
	}
	entries, err := readDotEnvFiles(files)
	if err != nil {
		return nil, err
	}
	env, expandable, origins := mergeDotEnvEntries(entries)
	for _, kv := range os.Environ() {
		key, value, _ := strings.Cut(kv, "=")
		env[key] = value
		delete(expandable, key)
	}
	if err := expandDotEnv(env, expandable, origins); err != nil {
		return nil, err
	}
	return env, nil
}

// readDotEnvFiles parses every listed file; a missing or unreadable file is
// an error (callers probe existence first when discovery is best-effort).
func readDotEnvFiles(files []string) ([]dotEnvEntry, error) {
	var out []dotEnvEntry
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("config: read %q: %w", file, err)
		}
		if len(data) > MaxDotEnvFileSize {
			return nil, fmt.Errorf("config: read %q: file too large", file)
		}
		entries, err := parseDotEnv(data, file)
		if err != nil {
			return nil, err
		}
		out = append(out, entries...)
	}
	return out, nil
}

// mergeDotEnvEntries folds parsed entries into one map in file order: later
// entries win. It also returns the keys whose values still need ${VAR}
// expansion and each key's file:line origin for error paths.
func mergeDotEnvEntries(entries []dotEnvEntry) (Env, map[string]bool, map[string]string) {
	env := make(Env, len(entries))
	expandable := make(map[string]bool, len(entries))
	origins := make(map[string]string, len(entries))
	for _, e := range entries {
		env[e.key] = e.value
		origins[e.key] = e.origin
		if e.expand {
			expandable[e.key] = true
		} else {
			delete(expandable, e.key)
		}
	}
	return env, expandable, origins
}

// parseDotEnv parses dotenv content line by line. Supported lines:
//
//	KEY=VALUE, KEY = VALUE (surrounding whitespace trimmed)
//	export KEY=VALUE (optional prefix)
//	KEY='literal' (single quotes: no escapes, no ${} expansion)
//	KEY="escaped" (double quotes: \n \t \r \\ \" \' \$ escapes, ${} expands)
//	KEY=unquoted # comment (a `#` preceded by space/tab starts a comment;
//	unquoted values otherwise keep `#` and expand ${})
//
// Blank lines and `#` comment lines are skipped. Anything else — a line
// without `=`, a bad key, an unterminated quote, trailing junk after a
// closing quote, an unknown escape, or a malformed `${` — fails closed with
// the file and line number. Values are never echoed into errors.
func parseDotEnv(data []byte, source string) ([]dotEnvEntry, error) {
	var out []dotEnvEntry
	lines := strings.Split(string(data), "\n")
	for i, raw := range lines {
		origin := fmt.Sprintf("%s:%d", source, i+1)
		line := strings.TrimSuffix(raw, "\r")
		if len(line) > MaxDotEnvLineLength {
			return nil, fmt.Errorf("config: parse %s: line too long", origin)
		}
		s := strings.TrimSpace(line)
		if s == "" || s[0] == '#' {
			continue
		}
		if rest, ok := strings.CutPrefix(s, "export"); ok && (rest == "" || rest[0] == ' ' || rest[0] == '\t') {
			s = strings.TrimSpace(rest)
		}
		key, value, ok := strings.Cut(s, "=")
		if !ok {
			return nil, fmt.Errorf("config: parse %s: line without KEY=VALUE", origin)
		}
		key = strings.TrimSpace(key)
		if !isDotEnvKey(key) {
			return nil, fmt.Errorf("config: parse %s: invalid key", origin)
		}
		parsed, expand, reason := parseDotEnvValue(strings.TrimLeft(value, " \t"))
		if reason != "" {
			return nil, fmt.Errorf("config: parse %s: %s", origin, reason)
		}
		out = append(out, dotEnvEntry{key: key, value: parsed, expand: expand, origin: origin})
	}
	return out, nil
}

// parseDotEnvValue parses one raw value: quoted when it opens with a quote,
// unquoted otherwise. It reports the parsed value, whether ${VAR}
// expansion applies (double-quoted and unquoted values expand;
// single-quoted values stay literal), and a non-empty failure reason.
// Reasons are plain text, never errors: the caller attributes them with the
// file and line.
func parseDotEnvValue(s string) (string, bool, string) {
	if s == "" {
		return "", true, ""
	}
	switch s[0] {
	case '\'':
		end := strings.IndexByte(s[1:], '\'')
		if end < 0 {
			return "", false, "unterminated single quote"
		}
		rest := strings.TrimSpace(s[1+end+1:])
		if rest != "" && !strings.HasPrefix(rest, "#") {
			return "", false, "trailing characters after closing quote"
		}
		return s[1 : 1+end], false, ""
	case '"':
		var sb strings.Builder
		i := 1
		closed := false
		for i < len(s) {
			c := s[i]
			if c == '"' {
				closed = true
				i++
				break
			}
			if c == '\\' {
				i++
				if i >= len(s) {
					return "", false, "trailing backslash"
				}
				switch s[i] {
				case 'n':
					sb.WriteByte('\n')
				case 't':
					sb.WriteByte('\t')
				case 'r':
					sb.WriteByte('\r')
				case '\\', '"', '\'', '$':
					sb.WriteByte(s[i])
				default:
					return "", false, "unknown escape"
				}
				i++
				continue
			}
			sb.WriteByte(c)
			i++
		}
		if !closed {
			return "", false, "unterminated double quote"
		}
		rest := strings.TrimSpace(s[i:])
		if rest != "" && !strings.HasPrefix(rest, "#") {
			return "", false, "trailing characters after closing quote"
		}
		return sb.String(), true, ""
	default:
		v := strings.TrimRight(s, " \t")
		if idx := dotEnvCommentIndex(v); idx >= 0 {
			v = strings.TrimRight(v[:idx], " \t")
		}
		return v, true, ""
	}
}

// dotEnvCommentIndex returns the index of a `#` comment opener in an
// unquoted value: the first `#` preceded by a space or tab. A `#` without
// preceding whitespace is literal value text.
func dotEnvCommentIndex(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '#' && i > 0 && (s[i-1] == ' ' || s[i-1] == '\t') {
			return i
		}
	}
	return -1
}

// isDotEnvKey reports whether s is a valid dotenv key: [A-Za-z_][A-Za-z0-9_]*.
func isDotEnvKey(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || i > 0 && c >= '0' && c <= '9' {
			continue
		}
		return false
	}
	return true
}

// expandDotEnv resolves ${VAR} references in the expandable values against
// the merged map, in passes up to DotEnvMaxExpansionDepth. Unset names
// expand to empty. Values that still reference after the depth bound (for
// example self-referential values) fail closed naming the file and line.
func expandDotEnv(env Env, expandable map[string]bool, origins map[string]string) error {
	keys := make([]string, 0, len(expandable))
	for key := range expandable {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for pass := 0; pass < DotEnvMaxExpansionDepth; pass++ {
		pending := false
		for _, key := range keys {
			out, reason := expandDotEnvValue(env[key], env)
			if reason != "" {
				return fmt.Errorf("config: parse %s: %s", origins[key], reason)
			}
			env[key] = out
			if strings.Contains(out, "${") {
				pending = true
			}
		}
		if !pending {
			return nil
		}
	}
	for _, key := range keys {
		if strings.Contains(env[key], "${") {
			return fmt.Errorf("config: parse %s: expansion exceeds max depth", origins[key])
		}
	}
	return nil
}

// expandDotEnvValue substitutes ${NAME} references from lookup. Only the
// braced form expands: `$`, `$(...)`, and backticks stay literal, so no
// subshell can ever run. An unterminated or badly named reference fails
// closed with a plain-text reason (the caller attributes it). Unset names
// expand to empty.
func expandDotEnvValue(s string, lookup Env) (string, string) {
	if !strings.Contains(s, "${") {
		return s, ""
	}
	var sb strings.Builder
	sb.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] == '$' && i+1 < len(s) && s[i+1] == '{' {
			rest := s[i+2:]
			end := strings.IndexByte(rest, '}')
			if end < 0 {
				return "", "unterminated ${ reference"
			}
			name := rest[:end]
			if !isDotEnvKey(name) {
				return "", "invalid ${ reference"
			}
			sb.WriteString(lookup[name])
			i += 2 + end + 1
			continue
		}
		sb.WriteByte(s[i])
		i++
	}
	return sb.String(), ""
}
