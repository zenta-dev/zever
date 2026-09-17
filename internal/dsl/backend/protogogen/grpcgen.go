package protogogen

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/pluginpb"
)

// grpcGoToolName is the Go tool name protoc-gen-go-grpc registers as, per the
// "tool google.golang.org/grpc/cmd/protoc-gen-go-grpc" directive added to
// go.mod: Go 1.24+'s tool-dependency tracking pins the module version and
// makes "go tool protoc-gen-go-grpc" build/run a reproducible binary with no
// separate install step.
const grpcGoToolName = "protoc-gen-go-grpc"

// protoMarshal is a test seam for proto.Marshal: CodeGeneratorRequest is a
// proto2 message with no required fields, so proto.Marshal on the valid
// request generateGRPCGo builds never fails via the public API (UTF-8
// enforcement applies to proto3 only, and FileToGenerate/Parameter accept
// arbitrary bytes). Swapping this seam is the only way to cover the marshal
// error branch.
var protoMarshal = proto.Marshal

// generateGRPCGo runs the real protoc-gen-go-grpc plugin as a pinned
// Go-tool subprocess. Unlike protoc-gen-go, protoc-gen-go-grpc's generator
// function is unexported inside package main
// (google.golang.org/grpc/cmd/protoc-gen-go-grpc) and genuinely cannot be
// imported as a library -- a real subprocess, speaking the same
// CodeGeneratorRequest/CodeGeneratorResponse protobuf wire protocol every
// protoc plugin implements over stdin/stdout, is the only faithful option.
// req is the exact same request generatePBGo consumes: one descriptor-
// building path feeds both plugin invocations.
func generateGRPCGo(ctx context.Context, req *pluginpb.CodeGeneratorRequest) (map[string][]byte, error) {
	reqBytes, err := protoMarshal(req)
	if err != nil {
		return nil, fmt.Errorf("[protogogen] marshal CodeGeneratorRequest: %w", err)
	}

	cmd := exec.CommandContext(ctx, "go", "tool", grpcGoToolName)
	cmd.Stdin = bytes.NewReader(reqBytes)

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("[protogogen] run %q: %w (stderr: %s)", grpcGoToolName, err, stderr.String())
	}

	var resp pluginpb.CodeGeneratorResponse
	if err := proto.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("[protogogen] unmarshal %q response: %w", grpcGoToolName, err)
	}

	if resp.GetError() != "" {
		return nil, fmt.Errorf("[protogogen] %s: %s", grpcGoToolName, resp.GetError())
	}

	return responseFilesToMap(&resp), nil
}
