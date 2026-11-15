package main

import (
	"os/exec"
	"strconv"
)

func configureRecipeProcess(command *exec.Cmd) {
	command.Cancel = func() error {
		return exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(command.Process.Pid)).Run()
	}
}
