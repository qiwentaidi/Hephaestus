//go:build !windows && !darwin

package main

import (
	"log"
	"os/exec"
)

func ensureDesktopRuntimePrerequisites() error {
	return nil
}

func reportDesktopStartupFailure(err error) {
	if err == nil {
		return
	}
	if message, ok := desktopPostgresConnectionFailureMessage(err); ok {
		showLinuxStartupError(message)
	}
	log.Fatal(err)
}

func showLinuxStartupError(message string) {
	for _, command := range []struct {
		name string
		args []string
	}{
		{"zenity", []string{"--error", "--title=Hephaestus", "--text=" + message}},
		{"kdialog", []string{"--title", "Hephaestus", "--error", message}},
	} {
		if _, err := exec.LookPath(command.name); err != nil {
			continue
		}
		if err := exec.Command(command.name, command.args...).Run(); err != nil {
			log.Printf("failed to show desktop startup error dialog with %s: %v", command.name, err)
			continue
		}
		return
	}
}
