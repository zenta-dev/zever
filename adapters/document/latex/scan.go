package latex

import (
	"fmt"
	"strings"
)

// pathCommands read or write a file named by their argument.
var pathCommands = map[string]bool{
	"input": true, "include": true, "includeonly": true, "InputIfFileExists": true,
	"verbatiminput": true, "lstinputlisting": true,
}

// streamCommands take a stream number before the file name.
var streamCommands = map[string]bool{"openin": true, "openout": true}

// shellPackages enable shell escape or external program execution.
var shellPackages = map[string]bool{
	"shellesc": true, "minted": true, "pythontex": true, "sagetex": true,
	"gnuplottex": true, "bashful": true, "write18": true, "epstopdf": true,
	"svg": true, "pstool": true, "auto-pst-pdf": true, "robust-externalize": true,
}

// scanSource rejects documents that name files outside the render directory
// (absolute, parent-relative, home-relative, or piped paths) or load
// shell-escape packages. It is a best-effort defense in depth: obfuscation
// such as \csname or \catcode tricks is not detected, so it never replaces
// the OS sandbox and -no-shell-escape.
func scanSource(src []byte) error {
	s := stripComments(string(src))
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			continue
		}
		j := i + 1
		for j < len(s) && isLetter(s[j]) {
			j++
		}
		name := s[i+1 : j]
		if name == "" {
			i++ // skip the escaped character (\\, \%, ...)
			continue
		}
		rest := s[j:]
		switch {
		case name == "write" && strings.HasPrefix(strings.TrimLeft(rest, " \t"), "18"):
			return unsafeErr(s, i, `\write18 shell escape`)
		case name == "usepackage" || name == "RequirePackage":
			if pkg, ok := badPackage(rest); ok {
				return unsafeErr(s, i, "shell-escape package "+pkg)
			}
		case pathCommands[name]:
			rest = strings.TrimPrefix(rest, "*")
			if p := fileArg(rest, true); !safePath(p) {
				return unsafeErr(s, i, fmt.Sprintf(`\%s path %q`, name, p))
			}
		case streamCommands[name]:
			if p := fileArg(skipStream(rest), false); !safePath(p) {
				return unsafeErr(s, i, fmt.Sprintf(`\%s path %q`, name, p))
			}
		}
		i = j - 1
	}
	return nil
}

func unsafeErr(s string, off int, what string) error {
	line := 1 + strings.Count(s[:off], "\n")
	return fmt.Errorf("%w: %s at line %d", ErrUnsafeSource, what, line)
}

func isLetter(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b == '@'
}

// stripComments removes unescaped % comments, keeping newlines.
func stripComments(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '\\' && i+1 < len(s):
			b.WriteByte(s[i])
			i++
			b.WriteByte(s[i])
		case s[i] == '%':
			for i < len(s) && s[i] != '\n' {
				i++
			}
			if i < len(s) {
				b.WriteByte('\n')
			}
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

func skipSpace(s string) string { return strings.TrimLeft(s, " \t\r\n") }

// skipStream skips "5" or "\mystream" and an optional "=".
func skipStream(s string) string {
	s = skipSpace(s)
	switch {
	case s != "" && s[0] >= '0' && s[0] <= '9':
		s = strings.TrimLeft(s, "0123456789")
	case strings.HasPrefix(s, "\\"):
		k := 1
		for k < len(s) && isLetter(s[k]) {
			k++
		}
		s = s[k:]
	}
	s = skipSpace(s)
	return skipSpace(strings.TrimPrefix(s, "="))
}

// fileArg extracts a braced or bare file argument, optionally skipping a
// leading [options] group. It returns "" when none can be read (for example a
// macro argument), which is treated as safe.
func fileArg(s string, allowOpt bool) string {
	s = skipSpace(s)
	if allowOpt && strings.HasPrefix(s, "[") {
		if k := strings.IndexByte(s, ']'); k >= 0 {
			s = skipSpace(s[k+1:])
		}
	}
	if strings.HasPrefix(s, "{") {
		depth := 0
		for k := 0; k < len(s); k++ {
			switch s[k] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					return strings.TrimSpace(s[1:k])
				}
			}
		}
		return strings.TrimSpace(s[1:])
	}
	k := 0
	for k < len(s) && !strings.ContainsRune(" \t\r\n{}\\", rune(s[k])) {
		k++
	}
	return s[:k]
}

func safePath(p string) bool {
	p = strings.Trim(p, `"' `)
	switch {
	case p == "":
		return true
	case strings.ContainsAny(p, "~|") || strings.Contains(p, ".."):
		return false
	case p[0] == '/' || p[0] == '\\':
		return false
	case len(p) >= 2 && p[1] == ':' && isLetter(p[0]):
		return false
	}
	return true
}

func badPackage(rest string) (string, bool) {
	rest = skipSpace(rest)
	if strings.HasPrefix(rest, "[") {
		if k := strings.IndexByte(rest, ']'); k >= 0 {
			rest = skipSpace(rest[k+1:])
		}
	}
	for _, pkg := range strings.Split(fileArg(rest, false), ",") {
		pkg = strings.TrimSpace(pkg)
		if shellPackages[pkg] {
			return pkg, true
		}
	}
	return "", false
}
