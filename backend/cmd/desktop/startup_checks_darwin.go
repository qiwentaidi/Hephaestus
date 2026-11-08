//go:build darwin

package main

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
)

func ensureDesktopRuntimePrerequisites() error {
	return nil
}

func reportDesktopStartupFailure(err error) {
	if err == nil {
		return
	}

	log.Printf("desktop startup failed: %v", err)
	if message, ok := desktopPostgresConnectionFailureMessage(err); ok {
		showDarwinStartupError(message)
	}
}

func showDarwinStartupError(message string) {
	script := fmt.Sprintf(
		`display alert %s message %s as critical buttons {"好"} default button "好"`,
		appleScriptString("Hephaestus"),
		appleScriptString(message),
	)
	if output, err := exec.Command("osascript", "-e", script).CombinedOutput(); err != nil {
		log.Printf("failed to show desktop startup error dialog: %v: %s", err, strings.TrimSpace(string(output)))
	}
}

func appleScriptString(value string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"\r", "",
		"\n", `\n`,
	)
	return `"` + replacer.Replace(value) + `"`
}
