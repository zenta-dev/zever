//go:build unix

package latex

import (
	"os/exec"
	"syscall"
)

// withGroupKill arranges for ctx cancellation to SIGKILL the child's whole
// process group: TeX may spawn helpers (kpsewhich, mktexfmt), and killing
// only the direct child would let a pipe-holding grandchild keep Render
// blocked in Wait long after the deadline.
func withGroupKill(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = ioWaitDelay
}
