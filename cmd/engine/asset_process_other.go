//go:build !unix && !windows && !js

package main

import "os/exec"

func configureRecipeProcess(_ *exec.Cmd) {}
