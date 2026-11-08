//go:build !windows

package mcp

import "os/exec"

func configureMCPWorkbenchCommand(cmd *exec.Cmd) {}
