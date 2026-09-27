package protogogen

import (
	"fmt"

	gengo "google.golang.org/protobuf/cmd/protoc-gen-go/internal_gengo"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/types/pluginpb"
)

// generatePBGo drives protoc-gen-go's real code generator as an in-process
// library (internal_gengo.GenerateFile is a plain exported function -- the
// directory is named internal_gengo, not internal, so Go's internal-package
// visibility rule does not block importing it from outside the
// google.golang.org/protobuf module). No subprocess is involved: req is the
// exact same *pluginpb.CodeGeneratorRequest real protoc would build before
// invoking protoc-gen-go over stdin/stdout, handed to protogen directly.
func generatePBGo(req *pluginpb.CodeGeneratorRequest) (map[string][]byte, error) {
	gen, err := (&protogen.Options{}).New(req)
	if err != nil {
		return nil, fmt.Errorf("[protogogen] protogen.Options.New: %w", err)
	}

	for _, f := range gen.Files {
		if f.Generate {
			gengo.GenerateFile(gen, f)
		}
	}

	resp := gen.Response()
	if resp.GetError() != "" {
		return nil, fmt.Errorf("[protogogen] protoc-gen-go: %s", resp.GetError())
	}

	return responseFilesToMap(resp), nil
}

// responseFilesToMap pulls Name/Content out of a CodeGeneratorResponse's
// generated files, shared by both the in-process and subprocess codegen
// paths.
func responseFilesToMap(resp *pluginpb.CodeGeneratorResponse) map[string][]byte {
	out := make(map[string][]byte, len(resp.GetFile()))

	for _, f := range resp.GetFile() {
		out[f.GetName()] = []byte(f.GetContent())
	}

	return out
}
