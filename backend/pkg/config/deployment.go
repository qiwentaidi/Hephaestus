package config

import (
	"os"
	"strings"
)

// DeploymentMode describes how the current process is hosted.
const (
	DeploymentModeNative  = "native"
	DeploymentModeDocker  = "docker"
	DeploymentModeDesktop = "desktop"
)

// IsContainerDeployment reports whether the process runs inside a managed
// container. Container images must be upgraded by replacing the image, not by
// self-replacing the binary in an ephemeral container layer.
func IsContainerDeployment() bool {
	if GetRuntimeMode() == RuntimeModeDesktop {
		return false
	}
	if strings.TrimSpace(os.Getenv("HEPHAESTUS_CONTAINER")) == "1" {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("HEPHAESTUS_DEPLOYMENT"))) {
	case "docker", "container", "kubernetes", "k8s", "podman":
		return true
	default:
		return false
	}
}

// GetDeploymentMode returns the deployment label exposed to clients.
func GetDeploymentMode() string {
	if GetRuntimeMode() == RuntimeModeDesktop {
		return DeploymentModeDesktop
	}
	if IsContainerDeployment() {
		return DeploymentModeDocker
	}
	return DeploymentModeNative
}
