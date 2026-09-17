//go:build unix

package main

import (
	"os"
	"os/exec"
	"syscall"
)

// isolateProcessGroup puts the `go run` child — and therefore the binary it
// compiles and execs — into its own process group, so forwardSignal can reach
// the whole tree with one kill. It also detaches the tree from the terminal's
// foreground group, which is what makes forwarding the *only* delivery path
// and keeps a Ctrl-C from racing us.
func isolateProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}

	cmd.SysProcAttr.Setpgid = true
}

// forwardSignal delivers sig to the child's whole process group. `go run`
// deliberately ignores SIGINT itself, so the signal has to reach the compiled
// binary it spawned; group delivery is how that happens.
func forwardSignal(cmd *exec.Cmd, sig os.Signal) {
	if cmd.Process == nil {
		return
	}

	sysSig, ok := sig.(syscall.Signal)
	if !ok {
		_ = cmd.Process.Signal(sig)
		return
	}

	// Setpgid made the child its own group leader, so its pid is the pgid.
	if err := syscall.Kill(-cmd.Process.Pid, sysSig); err != nil {
		_ = cmd.Process.Signal(sig)
	}
}

// killProcessGroup is the last resort `zever dev` reaches for when a child has
// not exited within the grace period after a forwarded SIGTERM. Like
// forwardSignal it targets the whole group, so a wedged compiled binary goes
// down along with the `go run` that spawned it.
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}

	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		_ = cmd.Process.Kill()
	}
}
