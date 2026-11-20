//go:build windows

package tunnel

import (
	"os/exec"
	"syscall"
)

func configureHiddenProcess(cmd *exec.Cmd) {
	const createNoWindow = 0x08000000
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: createNoWindow,
	}
}
