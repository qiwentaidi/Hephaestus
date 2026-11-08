//go:build windows

package portscan

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

func preparePingCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}
