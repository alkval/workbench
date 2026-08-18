//go:build !windows

package system

import "os/exec"

func configureCommand(_ *exec.Cmd) {}
