// Package embed provides a filesystem-backed i18n adapter.
//
// Catalogs are flat JSON objects (map[string]string) loaded from *.json
// files in a directory of an fs.FS. The file base name (minus .json) is
// the locale (e.g. en.json, fr-CA.json).
//
// All templates are parsed at load time (fail fast); parsed
// *template.Template values are cached and Execute is goroutine-safe, so
// Translate may run concurrently. Templates use missingkey=error: a
// missing argument fails the lookup instead of rendering silently.
//
// Templates ship from the deployer's own FS and are trusted; message
// arguments are data only and never executed as templates.
package embed
