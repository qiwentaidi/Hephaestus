package config

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	// ConfigUpdateVersionFileName 记录已应用的配置/PoC 包版本。
	ConfigUpdateVersionFileName = ".hephaestus-config-version"
	// DefaultConfigVersion 未记录版本时使用。
	DefaultConfigVersion = "v0.0.0"
)

// ReadConfigUpdateVersion 读取当前配置包版本，缺失或为空时返回默认版本。
func ReadConfigUpdateVersion() string {
	data, err := os.ReadFile(filepath.Join(GetConfigResourceRoot(), ConfigUpdateVersionFileName))
	if err != nil {
		return DefaultConfigVersion
	}
	if value := strings.TrimSpace(string(data)); value != "" {
		return value
	}
	return DefaultConfigVersion
}

// WriteConfigUpdateVersion 记录已应用的配置包版本。
func WriteConfigUpdateVersion(version string) error {
	path := filepath.Join(GetConfigResourceRoot(), ConfigUpdateVersionFileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.TrimSpace(version)+"\n"), 0o644)
}
