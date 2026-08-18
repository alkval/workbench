//go:build windows

package process

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/alkval/workbench/internal/config"
	ps "github.com/shirou/gopsutil/v4/process"
)

const (
	createNewProcessGroup = 0x00000200
	createNoWindow        = 0x08000000
)

func configureProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: createNewProcessGroup | createNoWindow,
		HideWindow:    true,
	}
}

func terminateTree(cmd *exec.Cmd, timeout time.Duration) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return terminatePIDTree(cmd.Process.Pid, timeout)
}

func terminatePIDTree(pid int, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	stop := exec.CommandContext(ctx, "taskkill.exe", "/PID", itoa(pid), "/T", "/F")
	stop.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	return stop.Run()
}

func knownProcessIDs(service config.Service) ([]int, error) {
	processes, err := ps.Processes()
	if err != nil {
		return nil, err
	}
	expectedExecutable := strings.ToLower(filepath.Clean(service.Command))
	ids := make([]int, 0, 1)
	for _, candidate := range processes {
		executable, exeErr := candidate.Exe()
		if exeErr != nil || strings.ToLower(filepath.Clean(executable)) != expectedExecutable {
			continue
		}
		commandLine, commandErr := candidate.Cmdline()
		if commandErr != nil || !containsConfiguredArguments(commandLine, service.Args) {
			continue
		}
		ids = append(ids, int(candidate.Pid))
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("no process matched %s and its configured arguments", service.Command)
	}
	return ids, nil
}

func containsConfiguredArguments(commandLine string, arguments []string) bool {
	for _, argument := range arguments {
		if argument != "" && !strings.Contains(strings.ToLower(commandLine), strings.ToLower(argument)) {
			return false
		}
	}
	return true
}
