//go:build unix

package main

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// go run owns a child executable. Cancel the whole group so a failed sibling
// recipe or an interrupted preview cannot leave processing/publishers running.
func configureRecipeProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}

		return err
	}
}
