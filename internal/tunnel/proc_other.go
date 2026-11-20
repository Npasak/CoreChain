//go:build !windows

package tunnel

import "os/exec"

func configureHiddenProcess(cmd *exec.Cmd) {}
