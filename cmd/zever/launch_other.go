//go:build !unix

package main

import (
	"os"
	"os/exec"
)

// isolateProcessGroup is a no-op off Unix: process groups are a POSIX concept.
func isolateProcessGroup(*exec.Cmd) {}

// forwardSignal degrades to signalling the immediate child. Reliable signal
// delivery through `go run` to the binary it spawns is out of scope off Unix —
// see the note in launch.go.
func forwardSignal(cmd *exec.Cmd, sig os.Signal) {
	if cmd.Process == nil {
		return
	}

	_ = cmd.Process.Signal(sig)
}

// killProcessGroup degrades to killing the immediate child off Unix.
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}

	_ = cmd.Process.Kill()
}
