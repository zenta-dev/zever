//go:build !unix

package latex

import "os/exec"

// withGroupKill kills the direct child on ctx cancellation. Process groups
// are not portable here, so helper grandchildren are not reaped; WaitDelay
// still bounds how long Wait lingers on their pipes.
func withGroupKill(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		return cmd.Process.Kill()
	}
	cmd.WaitDelay = ioWaitDelay
}
