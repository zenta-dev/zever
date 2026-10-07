package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"
)

// DefaultExecTimeout bounds `zever` subprocess calls from MCP tools.
const DefaultExecTimeout = 5 * time.Minute

// findZever locates the zever CLI binary on PATH.
func findZever() (string, error) {
	path, err := exec.LookPath("zever")
	if err != nil {
		return "", fmt.Errorf("zever-mcp: zever binary not on PATH: %w", err)
	}

	return path, nil
}

// runZeverIn executes the zever CLI with args in dir (empty dir means the
// current directory), returning combined output. A non-zero exit returns an
// error carrying the output for diagnosis.
func runZeverIn(ctx context.Context, dir string, args ...string) (string, error) {
	if len(args) == 0 {
		return "", errors.New("zever-mcp: no command")
	}

	bin, err := findZever()
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(ctx, DefaultExecTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	if err := cmd.Run(); err != nil {
		return buf.String(), fmt.Errorf("zever-mcp: zever %s: %w", args[0], err)
	}

	return buf.String(), nil
}
