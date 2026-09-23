// Package locales embeds the demo i18n message catalogs.
//
// The catalogs live in this directory so the go:embed pattern stays local
// (parent-directory patterns are rejected). internal/app wires FS into the
// i18n embed adapter.
package locales

import "embed"

//go:embed *.json

// FS holds en.json and fr.json.
var FS embed.FS
