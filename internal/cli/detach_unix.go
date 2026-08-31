//go:build !windows

package cli

import (
	"os/exec"
	"syscall"
)

// detach starts cmd in its own session so it survives the parent CLI
// process exiting (otherwise it would be killed along with the terminal's
// process group when the `otelma pull`/`run`/`ps` invocation that spawned
// it returns).
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
