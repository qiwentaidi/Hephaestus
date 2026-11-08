//go:build !windows

package portscan

import "os/exec"

func preparePingCommand(cmd *exec.Cmd) {
}
