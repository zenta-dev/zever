// Package protogogen implements the "protogogen" backend.Backend: it takes
// the "proto" backend's rendered ".proto" text one step further into real Go,
// producing genuine "*.pb.go" (protobuf message types) and "*_grpc.pb.go"
// (gRPC service stubs) source -- the same output real protoc plus
// protoc-gen-go/protoc-gen-go-grpc would produce, without ever invoking a
// system protoc binary.
//
// The two output kinds use opposite codegen mechanisms because the two
// plugins have opposite importability:
//
//   - protoc-gen-go's real generator (google.golang.org/protobuf/cmd/
//     protoc-gen-go/internal_gengo) is a plain exported package -- driven
//     in-process as a library, no subprocess (see pbgen.go).
//   - protoc-gen-go-grpc's generator is an unexported function inside
//     package main and genuinely cannot be imported -- run as a pinned
//     "go tool protoc-gen-go-grpc" subprocess instead, speaking the same
//     CodeGeneratorRequest/CodeGeneratorResponse wire protocol any protoc
//     plugin implements over stdin/stdout (see grpcgen.go).
//
// Both paths share one buildCodeGeneratorRequest (request.go): the .proto
// text is compiled once, through protocompile, into real descriptors, and
// the resulting *pluginpb.CodeGeneratorRequest is handed to both plugins --
// exactly the struct real protoc builds before invoking any plugin.
package protogogen

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/internal/dsl/backend/proto"
	"github.com/zenta-dev/zever/internal/dsl/ir"
)

// Backend renders a resolved schema all the way through to real, compilable
// Go protobuf message types and gRPC service stubs.
type Backend struct {
	proto *proto.Backend
}

// New returns a new protogogen Backend that sources its ".proto" text from
// a bare proto.New() -- see NewWithAnnotationsGoPackageRoot's doc comment
// for when to use that instead.
func New() *Backend {
	return &Backend{proto: proto.New()}
}

// NewWithAnnotationsGoPackageRoot returns a new protogogen Backend that
// sources its ".proto" text from
// proto.NewWithAnnotationsGoPackageRoot(annotationsGoPackageRoot) instead
// of a bare proto.New(). protogogen compiles that .proto text into real Go
// via protocompile, so its emitted zengo/annotations.pb.go's own package
// declaration -- and therefore every other generated file's import of it --
// otherwise always carries proto.New()'s default go_package
// (zen-go's own module), the same bug NewWithPBImportRoot on the gogen
// backend exists to fix for gogen's generated code; this is protogogen's
// side of that same fix, since Generate below drives its own independent
// proto.Backend instance rather than sharing one with the CLI's separately
// constructed "proto" registry entry.
func NewWithAnnotationsGoPackageRoot(annotationsGoPackageRoot string) *Backend {
	return &Backend{proto: proto.NewWithAnnotationsGoPackageRoot(annotationsGoPackageRoot)}
}

// Name returns the backend's identifier.
func (b *Backend) Name() string {
	return "protogogen"
}

// Generate renders schema to ".proto" text via the proto backend, compiles
// that text into real descriptors, and generates real "*.pb.go" and
// "*_grpc.pb.go" Go source for every proto file the proto backend produced
// (one per ir.Module, plus the shared zengo/annotations.proto). Output paths
// mirror their source ".proto" path (protoc's "paths=source_relative"
// convention), matching the per-module directory layout the proto backend
// already established.
func (b *Backend) Generate(schema *ir.Schema) (map[string][]byte, error) {
	protoFiles, err := b.proto.Generate(schema)
	if err != nil {
		return nil, fmt.Errorf("[protogogen] render proto: %w", err)
	}

	req, err := buildCodeGeneratorRequest(protoFiles)
	if err != nil {
		return nil, err
	}

	pbOut, err := generatePBGo(req)
	if err != nil {
		return nil, err
	}

	grpcOut, err := generateGRPCGo(context.Background(), req)
	if err != nil {
		return nil, err
	}

	out := make(map[string][]byte, len(pbOut)+len(grpcOut))

	for path, content := range pbOut {
		out[path] = content
	}

	for path, content := range grpcOut {
		out[path] = content
	}

	return out, nil
}
