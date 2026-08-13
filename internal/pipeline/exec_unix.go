//go:build !windows

package pipeline

import (
	"os/exec"
	"syscall"
	"time"
)

// setupProcessKill makes cancellation take out the whole process group —
// m4b-style tools spawn ffmpeg children that a plain Kill would orphan.
func setupProcessKill(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// Negative pid signals the process group.
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	}
	cmd.WaitDelay = 10 * time.Second // SIGKILL after grace period
}
