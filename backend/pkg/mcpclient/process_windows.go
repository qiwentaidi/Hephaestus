//go:build windows

package mcpclient

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

func configureProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}

func stopProcess(cmd *exec.Cmd) { _ = cmd.Process.Kill() }
