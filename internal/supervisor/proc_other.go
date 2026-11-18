//go:build !windows

package supervisor

import "os/exec"

func configureHiddenProcess(cmd *exec.Cmd) {
	// No special flags needed on Unix
}
