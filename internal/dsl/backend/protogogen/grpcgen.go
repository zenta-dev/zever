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
	reqBytes, err := proto.Marshal(req)
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
