package resolver

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/zenta-dev/zever/internal/dsl/ast"
	"github.com/zenta-dev/zever/internal/dsl/diag"
	"github.com/zenta-dev/zever/internal/dsl/ir"
)

// moduleResult is Pass -1's output: the module map (keyed by moduleKey, which
// combines version and name so e.g. v1/iam and v2/iam never collide), a
// side-table recording which ir.Module each accepted leaf declaration
// belongs to, and the accepted leaf declarations themselves in
// source-declaration order (files in the order given, then each file's Decls
// in order).
type moduleResult struct {
	modules    map[string]*ir.Module
	declModule map[ast.Decl]*ir.Module
	leafDecls  []ast.Decl
}

// versionSegment matches a dir-derived API version segment: "v" followed by
// one or more digits (v1, v2, v10, ...).
var versionSegment = regexp.MustCompile(`^v[0-9]+$`)

// pathSegmentsUnderSchemaDir returns the directory segments between
// schemaDir and filePath's containing directory, in order -- e.g.
// schema/v1/iam/user.zen with schemaDir "schema" returns ["v1", "iam"];
// schema/app.zen returns nil (the file sits directly in schemaDir).
// Handles both a path relative to cwd's schemaDir and an absolute path
// containing a schemaDir component anywhere in it (e.g. when the caller
// compiled files by absolute path rather than one rooted at cwd).
func pathSegmentsUnderSchemaDir(schemaDir, filePath string) []string {
	if schemaDir == "" {
		schemaDir = "schema"
	}

	if rel, err := filepath.Rel(schemaDir, filePath); err == nil {
		if rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			parts := strings.Split(filepath.ToSlash(rel), "/")
			if len(parts) > 1 {
				return parts[:len(parts)-1]
			}

			return nil
		}
	}

	// Fallback: search for a schemaDir path component anywhere in filePath,
	// for absolute paths or paths outside cwd's schema dir.
	slashPath := filepath.ToSlash(filePath)
	marker := "/" + schemaDir + "/"

	idx := strings.Index(slashPath, marker)
	if idx < 0 {
		return nil
	}

	after := slashPath[idx+len(marker):]
	parts := strings.Split(after, "/")

	if len(parts) > 1 {
		return parts[:len(parts)-1]
	}

	return nil
}

// versionAndModuleForPath derives (version, module) from filePath relative
// to schemaDir. When the immediate subdirectory under schemaDir matches
// versionSegment (v1, v2, ...), it is consumed as the version and the next
// subdirectory becomes the module; deeper nesting beyond that is ignored
// (still that version/module), matching the pre-existing "immediate
// subdirectory only" module rule. When the immediate subdirectory does not
// look like a version, it is the module directly and version is "" --
// every unversioned schema resolves exactly as it did before this existed.
//
//	schema/v1/iam/user.zen     -> version "v1", module "iam"
//	schema/v1/iam/sub/x.zen    -> version "v1", module "iam" (deeper nesting ignored)
//	schema/v1/x.zen            -> version "v1", module ""    (file directly under the version dir)
//	schema/billing/invoices/x.zen -> version "",  module "billing" (unchanged)
//	schema/app.zen             -> version "",  module ""    (unchanged)
func versionAndModuleForPath(schemaDir, filePath string) (version, module string) {
	segs := pathSegmentsUnderSchemaDir(schemaDir, filePath)
	if len(segs) == 0 {
		return "", ""
	}

	if versionSegment.MatchString(segs[0]) {
		if len(segs) > 1 {
			return segs[0], segs[1]
		}

		return segs[0], ""
	}

	return "", segs[0]
}

// moduleKey combines version and module name into moduleResult.modules' map
// key, so a module name repeated across versions (v1/iam and v2/iam) never
// merges into the same *ir.Module. This only keeps the two modules' own
// decls (entities/services/...) separate; it does NOT make entity/message/
// job names version-scoped -- resolveSymbols' duplicate-declaration check
// and the entityByName/messageByName/jobsByName indexes built later in
// Resolve are flat across the whole compile, an existing invariant this
// feature does not change. Compiling two versions of the same module that
// happen to declare a same-named entity in one invocation still fails as a
// duplicate declaration; the realistic use this supports is compiling one
// version's schema dir at a time (schema/v1/iam/*.zen alone), not multiple
// versions side by side in a single compile.
func moduleKey(version, module string) string {
	return version + "\x00" + module
}

// resolveModules is Pass -1: it groups every top-level declaration into its
// owning ir.Module based on the file's directory under schemaDir.
// Files directly under schemaDir belong to the implicit ir.Module{Name: ""} (public sentinel).
// Immediate subdirectory determines module name (or version, then module --
// see versionAndModuleForPath); deeper nesting is ignored (still that
// module). A subdirectory named "public" is reserved and reported as an
// error -- this check applies to the module segment only, never to a
// version segment.
func resolveModules(files []*ast.File, schemaDir string) (*moduleResult, diag.List) {
	var diags diag.List

	result := &moduleResult{
		modules:    map[string]*ir.Module{},
		declModule: map[ast.Decl]*ir.Module{},
	}

	if schemaDir == "" {
		schemaDir = "schema"
	}

	for _, f := range files {
		if f == nil || f.Name == "" {
			continue
		}

		// Defensive: versionAndModuleForPath must not panic on any file name
		// or schemaDir, even when called with synthetic test fixtures that
		// use absolute paths, empty strings, or names containing "..".
		// pathSegmentsUnderSchemaDir already normalizes empty schemaDir to
		// "schema" and guards all Rel/Index operations, but this wrapper
		// ensures a malformed file name never escapes as a panic.
		version, moduleName := versionAndModuleForPath(schemaDir, f.Name)

		if moduleName == "public" {
			diags = append(diags, diag.Wrap("resolve", diag.Position{File: f.Name}, ErrInvalidAttribute,
				"module name %q is reserved (public is the implicit module sentinel)", moduleName))
			// Still assign to public sentinel to avoid cascading errors.
			moduleName = ""
		}

		key := moduleKey(version, moduleName)

		mod := result.modules[key]
		if mod == nil {
			mod = &ir.Module{Name: moduleName, Version: version}
			result.modules[key] = mod
		}

		for _, d := range f.Decls {
			if d == nil {
				continue
			}

			result.declModule[d] = mod
			result.leafDecls = append(result.leafDecls, d)
		}
	}

	// Ensure at least the public sentinel exists for an empty file set, matching the
	// original "pure monolith" behavior where Resolve(nil) returns one implicit module.
	if len(result.modules) == 0 {
		result.modules[moduleKey("", "")] = &ir.Module{Name: ""}
	}

	return result, diags
}
