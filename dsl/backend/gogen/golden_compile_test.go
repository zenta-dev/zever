package gogen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/resolver"
)

// TestGoldenOutputCompiles proves the golden generated module genuinely
// compiles -- not merely that it parses (TestGoldenOutputParsesAsGo) -- by
// writing the whole package to a temp Go module and running `go vet` on it
// against the real zever runtime batteries. The temp module replaces every
// workspace zever module with its in-repo copy and stubs the protogogen
// message package (github.com/zenta-dev/zever/gen), which is project output
// that does not exist inside this repo. Resolution is offline: GOPROXY=off
// with the local module cache, falling back to one networked retry only when
// the cache is cold (e.g. a fresh CI runner).
func TestGoldenOutputCompiles(t *testing.T) {
	root := workspaceRoot(t)
	dir := t.TempDir()

	writeStubGenPackage(t, dir)
	writeTempModule(t, dir, root)

	files := generateGolden(t)
	saga := generateSaga(t)
	files["app/saga.go"] = saga["app/saga.go"]

	for path, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(path))

		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("create temp dir: %v", err)
		}

		if err := os.WriteFile(full, content, 0o600); err != nil {
			t.Fatalf("write temp file %s: %v", full, err)
		}
	}

	if err := runGoVet(t, dir); err != nil {
		t.Fatalf("golden module does not compile: %v", err)
	}
}

// workspaceRoot returns the repo root (the go.work directory), resolved from
// this test's own package directory (dsl/backend/gogen).
func workspaceRoot(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	root := filepath.Clean(filepath.Join(wd, "..", "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.work")); err != nil {
		t.Fatalf("go.work not found above %s: %v", wd, err)
	}

	return root
}

// writeStubGenPackage writes the stub of protogogen's generated message
// package for the golden schema: every pb.<Type> alias target, the
// TaskServiceServer interface pair, the FullMethodName constants, and
// RegisterTaskServiceServer that the golden files reference. The message
// types carry the fields the golden handlers/validators access and satisfy
// proto.Message (ProtoReflect) so the golden protojson calls type-check.
func writeStubGenPackage(t *testing.T, dir string) {
	t.Helper()

	genDir := filepath.Join(dir, "gen")

	if err := os.MkdirAll(genDir, 0o755); err != nil {
		t.Fatalf("create gen dir: %v", err)
	}

	goMod := "module github.com/zenta-dev/zever/gen\n\ngo 1.27.2\n\nrequire (\n" +
		"\tgoogle.golang.org/grpc v1.83.2\n\ngoogle.golang.org/protobuf v1.36.12\n)\n"

	if err := os.WriteFile(filepath.Join(genDir, "go.mod"), []byte(goMod), 0o600); err != nil {
		t.Fatalf("write gen go.mod: %v", err)
	}

	const genSrc = `// Package gen is a compile-test stub standing in for protogogen's
// generated message package (the real one is project output, absent from this
// repo). It mirrors exactly the pb symbols the golden files reference.
package gen

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type CreateTaskInput struct {
	Title string
	Email string
}

func (*CreateTaskInput) ProtoReflect() protoreflect.Message { return nil }

type Task struct{}

func (*Task) ProtoReflect() protoreflect.Message { return nil }

type GetTaskRequest struct {
	Id string
}

func (*GetTaskRequest) ProtoReflect() protoreflect.Message { return nil }

type ListTasksRequest struct {
	UserId string
	Cursor string
	Limit  int32
}

func (r *ListTasksRequest) GetCursor() string { return r.Cursor }

func (r *ListTasksRequest) GetLimit() int32 { return r.Limit }

func (*ListTasksRequest) ProtoReflect() protoreflect.Message { return nil }

type ListTasksResponse struct{}

func (*ListTasksResponse) ProtoReflect() protoreflect.Message { return nil }

type DeleteTaskRequest struct {
	Id string
}

func (*DeleteTaskRequest) ProtoReflect() protoreflect.Message { return nil }

type TaskServiceServer interface {
	CreateTask(context.Context, *CreateTaskInput) (*Task, error)
	GetTask(context.Context, *GetTaskRequest) (*Task, error)
	ListTasks(context.Context, *ListTasksRequest) (*ListTasksResponse, error)
	DeleteTask(context.Context, *DeleteTaskRequest) (*Task, error)
}

type UnimplementedTaskServiceServer struct{}

func (UnimplementedTaskServiceServer) CreateTask(context.Context, *CreateTaskInput) (*Task, error) {
	return nil, nil
}

func (UnimplementedTaskServiceServer) GetTask(context.Context, *GetTaskRequest) (*Task, error) {
	return nil, nil
}

func (UnimplementedTaskServiceServer) ListTasks(context.Context, *ListTasksRequest) (*ListTasksResponse, error) {
	return nil, nil
}

func (UnimplementedTaskServiceServer) DeleteTask(context.Context, *DeleteTaskRequest) (*Task, error) {
	return nil, nil
}

const (
	TaskService_CreateTask_FullMethodName   = "/task.TaskService/CreateTask"
	TaskService_GetTask_FullMethodName      = "/task.TaskService/GetTask"
	TaskService_ListTasks_FullMethodName    = "/task.TaskService/ListTasks"
	TaskService_DeleteTask_FullMethodName   = "/task.TaskService/DeleteTask"
)

func RegisterTaskServiceServer(grpc.ServiceRegistrar, TaskServiceServer) {}
`

	if err := os.WriteFile(filepath.Join(genDir, "gen.go"), []byte(genSrc), 0o600); err != nil {
		t.Fatalf("write gen stub: %v", err)
	}
}

// writeTempModule writes the temp module's go.mod: it requires the zever
// batteries the golden package imports plus grpc/protobuf, replaces every
// workspace zever module with its in-repo copy (the same set go.work's
// ALL_MODULES covers), and seeds go.sum from the workspace's own go.work.sum
// so verification needs no network.
func writeTempModule(t *testing.T, dir, root string) {
	t.Helper()

	modulePaths := workspaceModulePaths(t, root)

	var b strings.Builder

	b.WriteString("module goldentest\n\ngo 1.27.2\n\nrequire (\n")
	for _, req := range []string{
		"github.com/zenta-dev/zever/adapters/log/noop v0.6.1",
		"github.com/zenta-dev/zever/core/auth v0.6.1",
		"github.com/zenta-dev/zever/core/authz v0.6.1",
		"github.com/zenta-dev/zever/core/document v0.6.1",
		"github.com/zenta-dev/zever/core/log v0.6.1",
		"github.com/zenta-dev/zever/core/permission v0.6.1",
		"github.com/zenta-dev/zever/core/router v0.6.1",
		"github.com/zenta-dev/zever/core/workflow v0.6.1",
		"github.com/zenta-dev/zever/shared/apperror v0.6.1",
		"github.com/zenta-dev/zever/shared/codec v0.8.0",
		"github.com/zenta-dev/zever/gen v0.0.0",
		"google.golang.org/grpc v1.83.2",
		"google.golang.org/protobuf v1.36.12",
	} {
		b.WriteString("\t" + req + "\n")
	}

	b.WriteString(")\n\n")

	for _, mp := range modulePaths {
		rel, err := filepath.Rel(root, moduleDir(t, root, mp))
		if err != nil {
			t.Fatalf("resolve replace path for %s: %v", mp, err)
		}

		b.WriteString("replace " + mp + " => " + filepath.ToSlash(filepath.Join(root, filepath.FromSlash(rel))) + "\n")
	}

	b.WriteString("\nreplace github.com/zenta-dev/zever/gen => ./gen\n")

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(b.String()), 0o600); err != nil {
		t.Fatalf("write temp go.mod: %v", err)
	}

	seen := map[string]bool{}
	sum := new(strings.Builder)

	seedSumFile(t, sum, seen, filepath.Join(root, "go.work.sum"))

	for _, mp := range modulePaths {
		seedSumFile(t, sum, seen, filepath.Join(moduleDir(t, root, mp), "go.sum"))
	}

	if err := os.WriteFile(filepath.Join(dir, "go.sum"), []byte(sum.String()), 0o600); err != nil {
		t.Fatalf("write temp go.sum: %v", err)
	}
}

// seedSumFile appends every line of a go.sum file to b, skipping lines
// already seeded (identical lines are deduplicated) so merging many
// module go.sums stays idempotent.
func seedSumFile(t *testing.T, b *strings.Builder, seen map[string]bool, path string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}

		t.Fatalf("read sum file: %v", err)
	}

	for _, line := range strings.Split(string(data), "\n") {
		if line == "" || seen[line] {
			continue
		}

		seen[line] = true
		b.WriteString(line + "\n")
	}
}

// workspaceModulePaths returns the import path of every Go module under root
// (the same set the Makefile's ALL_MODULES covers: .git, .worktrees, and the
// two standalone out-of-workspace example modules excluded).
func workspaceModulePaths(t *testing.T, root string) []string {
	t.Helper()

	var paths []string

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			switch d.Name() {
			case ".git", ".worktrees":
				return filepath.SkipDir
			}

			if path == root {
				return nil
			}

			base := filepath.Base(path)
			if base == "external-sms" || base == "docs" && filepath.Base(filepath.Dir(path)) == "docs" {
				if _, statErr := os.Stat(filepath.Join(path, "go.mod")); statErr == nil {
					return filepath.SkipDir
				}
			}

			return nil
		}

		if d.Name() != "go.mod" {
			return nil
		}

		data, readErr := os.ReadFile(path) // path comes from filepath.WalkDir over the repo root, not user input
		if readErr != nil {
			return readErr
		}

		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "module ") {
				paths = append(paths, strings.TrimSpace(strings.TrimPrefix(line, "module ")))
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk workspace modules: %v", err)
	}

	return paths
}

// moduleDir returns the directory of the workspace module with the given
// import path. Most paths map by trimming the github.com/zenta-dev/zever
// prefix; the two standalone example modules documented in the Makefile's
// ALL_MODULES exceptions are special-cased.
func moduleDir(t *testing.T, root, modulePath string) string {
	t.Helper()

	rel := strings.TrimPrefix(modulePath, "github.com/zenta-dev/zever")
	if rel == "" {
		rel = "."
	}

	if modulePath == "github.com/zenta-dev/zever/docs-examples" {
		rel = "docs/examples"
	}

	if modulePath == "github.com/example/zever-sms" {
		rel = "examples/external-sms"
	}

	dir := filepath.Join(root, filepath.FromSlash(rel))

	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		t.Fatalf("module %s has no go.mod at %s", modulePath, dir)
	}

	return dir
}

// runGoVet runs `go vet ./...` in dir, offline first (GOPROXY=off, sums
// already seeded from go.work.sum), retrying once with the proxy enabled
// when the local module cache is cold.
func runGoVet(t *testing.T, dir string) error {
	t.Helper()

	env := append(os.Environ(),
		"GOWORK=off",
		"GOPROXY=off",
		"GOSUMDB=off",
		"GOFLAGS=-mod=mod",
	)

	cmd := exec.CommandContext(t.Context(), "go", "vet", "./...")
	cmd.Dir = dir
	cmd.Env = env

	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}

	if !strings.Contains(string(out), "module lookup disabled") && !strings.Contains(string(out), "missing go.sum entry") {
		return &vetError{out: out, err: err}
	}

	cmd = exec.CommandContext(t.Context(), "go", "vet", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOSUMDB=off", "GOFLAGS=-mod=mod")

	out, err = cmd.CombinedOutput()
	if err != nil {
		return &vetError{out: out, err: err}
	}

	return nil
}

// vetError carries go vet's combined output for the failure message.
type vetError struct {
	out []byte
	err error
}

func (e *vetError) Error() string {
	return "go vet: " + e.err.Error() + "\n" + string(e.out)
}

// generateSaga renders the saga fixture's saga.go, reusing the golden
// test's own schema pipeline.
func generateSaga(t *testing.T) map[string][]byte {
	t.Helper()

	file := compileSchema(t, sagaFixture)

	schema, diags := resolver.Resolve([]*ast.File{file})
	if diags.HasErrors() {
		t.Fatalf("resolve errors: %v", diags)
	}

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	return out
}
