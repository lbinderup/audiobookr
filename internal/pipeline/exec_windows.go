//go:build windows

package pipeline

import (
	"os/exec"
	"time"
)

// setupProcessKill on Windows: ffmpeg/tone are invoked directly (no wrapper
// shells), so killing the immediate child is sufficient for dev use.
func setupProcessKill(cmd *exec.Cmd) {
	cmd.WaitDelay = 10 * time.Second
}
