//go:build windows

package mcp

import (
	"os/exec"
	"syscall"
)

const mcpCreateNoWindow = 0x08000000

func configureMCPWorkbenchCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: mcpCreateNoWindow,
	}
}
