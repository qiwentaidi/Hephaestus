//go:build !windows

package admin

import "os/exec"

func prepareBackgroundCommand(cmd *exec.Cmd) {
}
