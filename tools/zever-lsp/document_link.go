package main

import (
	"regexp"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// commentURLPattern matches an http or https URL inside comment text: the
// scheme at the match's exact start, followed by a run of non-space
// characters. The pattern is deliberately permissive about the tail --
// punctuation caught up in comment prose is trimmed below -- and every
// candidate is re-checked to begin a word, so a scheme lookalike buried
// inside an identifier-length run can never match.
//
// The regexp is compiled once at package level because it is an immutable
// value (the repo-wide pattern tools like modgraph and apidiff follow),
// never per-request state.
var commentURLPattern = regexp.MustCompile(`https?://\S+`)

// documentLinksFor scans one document's line comments for http/https URLs
// and returns one DocumentLink per URL found. Links carry only their range
// and target: Tooltip is documentLink/resolve's to fill, exactly when the
// client negotiated tooltipSupport in Initialize.
//
// Only `//` comment text is ever scanned. The lexer eats comments without
// recording them anywhere (see format.go, which is built entirely around
// that fact), so the token stream carries no comment text and a plain
// line-by-line scan of the SOURCE is the only way to recover it. That scan
// also closes the boundary where an apparent comment is not one: a `//`
// inside a string literal -- a URL's own "http://host/..." spelling being a
// plausible one -- starts nothing, and since the DSL has no other '/'
// token at all, a slash pair outside a string is exactly a comment opener.
//
// The scheme is pinned to exactly http:// or https://, every remainder
// character is a non-space run, and trailing punctuation comment prose
// routinely leaves behind ('.', ',', ')', ';') is trimmed off the target so
// a link does not extend into surrounding sentences. A URL partially hidden
// in identifier text is rejected: the character immediately before the
// scheme must not be able to appear in an identifier.
func documentLinksFor(src string) []protocol.DocumentLink {
	links := []protocol.DocumentLink{}

	for line, text := range splitSourceLines(src) {
		start := commentStartAt(text)
		if start < 0 {
			continue // no comment on this line
		}

		links = appendCommentLinks(links, text, start, line)
	}

	return links
}

// appendCommentLinks appends one link per URL the comment region starting
// at byte offset begin of text carries, ranges addressed on the 0-based LSP
// row line. A candidate holding no text beyond the scheme after punctuation
// trimming adds nothing.
func appendCommentLinks(links []protocol.DocumentLink, text string, begin, line int) []protocol.DocumentLink {
	comment := text[begin:]

	for _, match := range commentURLPattern.FindAllStringIndex(comment, -1) {
		urlStart, urlEnd := begin+match[0], begin+match[1]

		// The scheme must start a word, not continue one: the character
		// immediately before it cannot open a link if it could be part of
		// the same identifier run ("xhttps://host" links nothing).
		if urlStart > 0 && isLinkWordByte(text[urlStart-1]) {
			continue
		}

		url := trimURLPunctuation(text[urlStart:urlEnd])
		if url == "" {
			continue
		}

		target := uri.URI(url)

		links = append(links, protocol.DocumentLink{
			Range:  commentLinkRange(text, urlStart, urlStart+len(url), line),
			Target: &target,
		})
	}

	return links
}

// commentStartAt returns the byte offset at which a `//` comment begins --
// the first slash pair outside a string literal -- or -1 when the line
// starts no comment. Unterminated strings are treated as string content to
// the end of the line, which errs towards NOT scanning, the conservative
// direction: a false comment start would be the only way a string-embedded
// URL could link.
func commentStartAt(text string) int {
	inString := false

	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '\\':
			if inString {
				i++ // every escape is backslash plus exactly one more byte
			}
		case '"':
			inString = !inString
		case '/':
			if !inString && i+1 < len(text) && text[i+1] == '/' {
				return i
			}
		}
	}

	return -1
}

// isLinkWordByte reports whether b can appear inside an identifier, the
// same ASCII rule the lexer's isIdentStart/isIdentContinue admits
// (position.go's isIdentByte twin), so scheme lookalikes attached to
// identifier text are rejected at their boundary.
func isLinkWordByte(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// trimURLPunctuation cuts the trailing punctuation comments routinely
// attach to a URL -- '.', ',', ')' and ';' -- one character at a time, so a
// sentence like "see https://example.com/docs." links the URL and not the
// period that ends the sentence. A candidate that trims to nothing
// (impossible for http(s) scheme-pinned matches, kept for robustness) is
// reported as empty and dropped by the caller.
func trimURLPunctuation(url string) string {
	for url != "" {
		switch url[len(url)-1] {
		case '.', ',', ')', ';':
			url = url[:len(url)-1]
		default:
			return url
		}
	}

	return url
}

// commentLinkRange builds a single-line link range from byte offsets,
// measured in the UTF-16 code units the LSP addresses ranges in.
func commentLinkRange(text string, start, end, line int) protocol.Range {
	return protocol.Range{
		Start: protocol.Position{
			//nolint:gosec // line indexes and measured widths are non-negative
			Line:      uint32(line),
			Character: uint32(lineWidth(text[:start])), //nolint:gosec // line widths non-negative
		},
		End: protocol.Position{
			//nolint:gosec // line indexes and measured widths are non-negative
			Line:      uint32(line),
			Character: uint32(lineWidth(text[:end])), //nolint:gosec // line widths non-negative
		},
	}
}
