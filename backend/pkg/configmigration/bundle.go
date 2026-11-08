// Package configmigration exports and imports portable Hephaestus
// configuration modules without touching runtime state that is already managed
// elsewhere. It currently covers AI providers, third-party API keys,
// notification channels and scan templates.
package configmigration

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	// BundleSchema identifies files produced by this package.
	BundleSchema = "hephaestus.config.migration"
	// BundleVersion is the current serialization version.
	BundleVersion = 1
)

const (
	ModuleAI            = "ai"
	ModuleAPIKeys       = "apiKeys"
	ModuleScanTemplates = "scanTemplates"
	ModuleNotifications = "notifications"
)

// moduleOrder controls both the export layout and the import application order.
var moduleOrder = []string{ModuleAI, ModuleAPIKeys, ModuleNotifications, ModuleScanTemplates}

// SupportedModules is the user-facing list of migration modules.
var SupportedModules = append([]string(nil), moduleOrder...)

// Bundle is the exported configuration file understood by the import endpoint.
type Bundle struct {
	Schema     string    `json:"schema"`
	Version    int       `json:"version"`
	ExportedAt time.Time `json:"exportedAt"`
	AppVersion string    `json:"appVersion,omitempty"`
	Encrypted  bool      `json:"encrypted"`
	Modules    []Module  `json:"modules"`
}

// Module is one independently selectable configuration module.
type Module struct {
	ID   string          `json:"id"`
	Data json.RawMessage `json:"data"`
}

// ImportRequest is the POST body for the import endpoint.
type ImportRequest struct {
	Bundle  Bundle   `json:"bundle"`
	Modules []string `json:"modules"`
}

// ModuleResult reports what an import did for one module.
type ModuleResult struct {
	ID      string `json:"id"`
	Applied int    `json:"applied"`
	Created int    `json:"created,omitempty"`
	Updated int    `json:"updated,omitempty"`
}

// ImportResult is returned after a successful import.
type ImportResult struct {
	Modules []ModuleResult `json:"modules"`
}

// ValidationError marks problems caused by the submitted bundle or selection.
// Handlers can map it to a 400 response instead of an internal error.
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

func invalid(format string, args ...any) error {
	return &ValidationError{Message: fmt.Sprintf(format, args...)}
}

func normalizeModules(requested []string) ([]string, error) {
	if len(requested) == 0 {
		return append([]string(nil), moduleOrder...), nil
	}
	seen := make(map[string]bool, len(requested))
	result := make([]string, 0, len(requested))
	for _, raw := range requested {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if !isSupportedModule(id) {
			return nil, invalid("不支持的配置模块: %s", id)
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		result = append(result, id)
	}
	if len(result) == 0 {
		return nil, invalid("请至少选择一个配置模块")
	}
	return result, nil
}

func isSupportedModule(id string) bool {
	for _, supported := range moduleOrder {
		if supported == id {
			return true
		}
	}
	return false
}

// ParseModules converts a comma-separated query value into module IDs.
func ParseModules(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
