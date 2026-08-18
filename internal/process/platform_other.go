//go:build !windows

package process

import (
	"errors"
	"os"
	"os/exec"
	"time"

	"github.com/alkval/workbench/internal/config"
)

func configureProcess(cmd *exec.Cmd) {}

func terminateTree(cmd *exec.Cmd, timeout time.Duration) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	_ = cmd.Process.Signal(os.Interrupt)
	time.Sleep(min(timeout, 2*time.Second))
	return cmd.Process.Kill()
}

func terminatePIDTree(_ int, _ time.Duration) error {
	return errors.New("stopping externally detected processes is only supported on Windows")
}

func knownProcessIDs(_ config.Service) ([]int, error) {
	return nil, errors.New("identifying externally detected processes is only supported on Windows")
}
