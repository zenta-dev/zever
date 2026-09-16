// Package proto implements the "proto" backend.Backend: it renders a
// resolved *ir.Schema into one Protocol Buffers file per ir.Module (plus the
// shared zengo/annotations.proto companion file), matching the field-type
// mapping, request-message synthesis, and http/auth/permission option
// rendering rules documented in the design doc and Task 10's brief.
package proto

import (
	_ "embed"
	"strings"

	"github.com/zenta-dev/zever/internal/dsl/ir"
)

// AnnotationsProtoSource is the verbatim source of the hand-authored
// zengo/annotations.proto companion file, embedded at build time and
// shipped as an output file by every Generate call.
//
//go:embed zengo/annotations.proto
var AnnotationsProtoSource string

// defaultAnnotationsGoPackageRoot is the Go import root baked into
// AnnotationsProtoSource's own "option go_package" line -- correct only for
// schemas compiled and consumed inside this module (see the committed
// gen/zengo/annotations package). Any other project that imports
// "zengo/annotations.proto" gets this exact companion file copied
// byte-for-byte into its own compiled output (Generate's
// out["zengo/annotations.proto"]), so protoc-gen-go would otherwise bake a
// go_package pointing back inside this framework's own module into that
// project's generated code, rather than into the project's own locally
// compiled copy of the same file.
const defaultAnnotationsGoPackageRoot = "github.com/zenta-dev/zen-go/gen/zengo/annotations"

// annotationsGoPackageLine is the exact "option go_package = ...;" line
// AnnotationsProtoSource declares, targeted by annotationsProtoSource for a
// minimal, surgical rewrite that never risks drifting from the rest of the
// embedded file (the auth/permission proto extensions it defines).
const annotationsGoPackageLine = `option go_package = "` + defaultAnnotationsGoPackageRoot + `;annotationsv1";`

// Backend renders a resolved schema to Protocol Buffers source files.
type Backend struct {
	annotationsGoPackageRoot string
}

// New returns a new proto Backend that emits the embedded
// zengo/annotations.proto companion file verbatim, with its go_package
// option pointing at defaultAnnotationsGoPackageRoot -- correct for schemas
// compiled and consumed inside this module, and a reasonable default
// anywhere else until overridden.
func New() *Backend {
	return &Backend{annotationsGoPackageRoot: defaultAnnotationsGoPackageRoot}
}

// NewWithAnnotationsGoPackageRoot returns a new proto Backend that rewrites
// the embedded zengo/annotations.proto companion file's own "option
// go_package" line to "<annotationsGoPackageRoot>;annotationsv1" instead of
// the default defaultAnnotationsGoPackageRoot. Use this when the target
// project compiles its own local copy of zengo/annotations.proto (e.g. at
// "<project>/generated/protogogen/zengo/annotations.pb.go") and needs every
// OTHER generated .pb.go file that imports "zengo/annotations.proto" to
// blank-import that project's own package, rather than a path inside this
// framework's own module -- Generate(schema) is never told the calling
// project's module path or --out layout, so an exact downstream import path
// can't be derived in general.
func NewWithAnnotationsGoPackageRoot(annotationsGoPackageRoot string) *Backend {
	if annotationsGoPackageRoot == "" {
		annotationsGoPackageRoot = defaultAnnotationsGoPackageRoot
	}

	return &Backend{annotationsGoPackageRoot: annotationsGoPackageRoot}
}

// Name returns the backend's identifier.
func (b *Backend) Name() string {
	return "proto"
}

// Generate renders one "<module>/schema.proto" file per schema.Modules
// entry (the implicit unnamed module renders to "schema.proto" at the
// output root) plus the shared "zengo/annotations.proto" companion file.
func (b *Backend) Generate(schema *ir.Schema) (map[string][]byte, error) {
	out := make(map[string][]byte, len(schema.Modules)+1)

	for _, m := range schema.Modules {
		path, content, err := renderModuleFile(m)
		if err != nil {
			return nil, err
		}

		out[path] = content
	}

	out["zengo/annotations.proto"] = b.annotationsProtoSource()

	return out, nil
}

// annotationsProtoSource returns AnnotationsProtoSource verbatim when b was
// constructed with New() (or an empty override), or with its "option
// go_package" line's import root rewritten to b.annotationsGoPackageRoot
// when constructed via NewWithAnnotationsGoPackageRoot. This targeted
// string replacement -- rather than hand-reconstructing the whole .proto
// file -- keeps the rest of AnnotationsProtoSource (the auth/permission
// proto extensions it defines) exactly as embedded, at no risk of drifting
// from source.
func (b *Backend) annotationsProtoSource() []byte {
	if b.annotationsGoPackageRoot == "" || b.annotationsGoPackageRoot == defaultAnnotationsGoPackageRoot {
		return []byte(AnnotationsProtoSource)
	}

	rewritten := `option go_package = "` + b.annotationsGoPackageRoot + `;annotationsv1";`

	return []byte(strings.Replace(AnnotationsProtoSource, annotationsGoPackageLine, rewritten, 1))
}
