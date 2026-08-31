//go:build windows

package cli

import "os/exec"

// detach is a no-op on Windows; auto-started `otelma serve` will exit when
// the parent process tree does. Not a target platform for v0.1 (see
// README), kept only so the package builds cross-platform.
func detach(cmd *exec.Cmd) {}
