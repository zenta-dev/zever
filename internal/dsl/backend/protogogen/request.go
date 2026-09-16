package protogogen

import (
	"context"
	_ "embed"
	"fmt"
	"sort"

	"github.com/bufbuild/protocompile"
	"github.com/bufbuild/protocompile/linker"
	"github.com/bufbuild/protocompile/wellknownimports"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

// googleAPIAnnotationsProto and googleAPIHTTPProto are verbatim, vendored
// copies of the two googleapis companion files the proto backend's own
// rendered output may import (google/api/annotations.proto,
// google/api/http.proto -- Apache-2.0, see the embedded files' own license
// headers). They are never generated into Go output themselves (they are
// not added to CodeGeneratorRequest.FileToGenerate below), only supplied so
// protocompile can resolve the import when a rendered module file declares
// an `http:` binding on an rpc.
var (
	//go:embed googleapis/google/api/annotations.proto
	googleAPIAnnotationsProto string

	//go:embed googleapis/google/api/http.proto
	googleAPIHTTPProto string
)

// codeGeneratorParameter is passed to both the in-process protoc-gen-go
// library call and the protoc-gen-go-grpc subprocess so that generated file
// names mirror their source ".proto" path exactly (e.g. "billing/schema.proto"
// -> "billing/schema.pb.go" / "billing/schema_grpc.pb.go") rather than being
// rewritten to sit under go_package's full Go import-path directory -- the
// same per-module directory layout convention the proto and gogen backends
// already use.
const codeGeneratorParameter = "paths=source_relative"

// buildCodeGeneratorRequest compiles protoFiles (the proto backend's own
// Generate output: one file per ir.Module plus the shared
// zengo/annotations.proto) into real descriptors via protocompile, then
// assembles the single *pluginpb.CodeGeneratorRequest that both the in-process
// protoc-gen-go path and the protoc-gen-go-grpc subprocess path consume --
// exactly the struct real protoc builds before invoking any plugin. Shared
// between both codegen paths per the design so descriptor-building logic
// exists exactly once.
func buildCodeGeneratorRequest(protoFiles map[string][]byte) (*pluginpb.CodeGeneratorRequest, error) {
	toGenerate := make([]string, 0, len(protoFiles))
	sources := make(map[string]string, len(protoFiles)+2)

	for path, content := range protoFiles {
		toGenerate = append(toGenerate, path)
		sources[path] = string(content)
	}

	sort.Strings(toGenerate)

	sources["google/api/annotations.proto"] = googleAPIAnnotationsProto
	sources["google/api/http.proto"] = googleAPIHTTPProto

	resolver := wellknownimports.WithStandardImports(&protocompile.SourceResolver{
		Accessor: protocompile.SourceAccessorFromMap(sources),
	})

	files, err := (&protocompile.Compiler{Resolver: resolver}).Compile(context.Background(), toGenerate...)
	if err != nil {
		return nil, fmt.Errorf("[protogogen] compile proto sources: %w", err)
	}

	fileDescriptors := collectFileDescriptorsInDependencyOrder(files)

	return &pluginpb.CodeGeneratorRequest{
		FileToGenerate: toGenerate,
		Parameter:      proto.String(codeGeneratorParameter),
		ProtoFile:      fileDescriptors,
	}, nil
}

// collectFileDescriptorsInDependencyOrder walks files and their transitive
// imports depth-first, emitting each *descriptorpb.FileDescriptorProto only
// once, in dependency-first order (a file's imports always appear before the
// file itself) -- the order both protoc-gen-go and protoc-gen-go-grpc expect
// a CodeGeneratorRequest.ProtoFile list to be in, matching real protoc's own
// behavior.
func collectFileDescriptorsInDependencyOrder(files linker.Files) []*descriptorpb.FileDescriptorProto {
	seen := make(map[string]bool)

	out := make([]*descriptorpb.FileDescriptorProto, 0, len(files))

	var visit func(fd protoreflect.FileDescriptor)

	visit = func(fd protoreflect.FileDescriptor) {
		if seen[fd.Path()] {
			return
		}

		seen[fd.Path()] = true

		imports := fd.Imports()
		for i := range imports.Len() {
			visit(imports.Get(i).FileDescriptor)
		}

		out = append(out, protodesc.ToFileDescriptorProto(fd))
	}

	for _, f := range files {
		visit(f)
	}

	return out
}
