package configmigration

import (
	"encoding/json"
	"fmt"
	"time"

	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/version"
)

// exportedScanTemplate is the portable representation of a scan template.
// IDs are intentionally omitted: import matches templates by name so a bundle
// stays valid across installations.
type exportedScanTemplate struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	TaskType    string          `json:"taskType"`
	Config      json.RawMessage `json:"config,omitempty"`
}

// Export collects the requested modules into a single portable bundle.
// Secret fields are encrypted with the compiled-in migration key.
func Export(requested []string) (Bundle, error) {
	modules, err := normalizeModules(requested)
	if err != nil {
		return Bundle{}, err
	}

	bundle := Bundle{
		Schema:     BundleSchema,
		Version:    BundleVersion,
		ExportedAt: time.Now().UTC(),
		AppVersion: version.Version,
		Encrypted:  true,
		Modules:    make([]Module, 0, len(modules)),
	}

	for _, id := range modules {
		data, err := exportModule(id)
		if err != nil {
			return Bundle{}, fmt.Errorf("导出模块 %s 失败: %w", id, err)
		}
		encrypted, err := encryptSecrets(data)
		if err != nil {
			return Bundle{}, fmt.Errorf("加密模块 %s 失败: %w", id, err)
		}
		raw, err := json.Marshal(encrypted)
		if err != nil {
			return Bundle{}, fmt.Errorf("序列化模块 %s 失败: %w", id, err)
		}
		bundle.Modules = append(bundle.Modules, Module{ID: id, Data: raw})
	}

	return bundle, nil
}

func exportModule(id string) (any, error) {
	switch id {
	case ModuleAI:
		aiConfig, err := config.GetAIConfig()
		if err != nil {
			return nil, err
		}
		return toJSONValue(aiConfig)
	case ModuleAPIKeys:
		spaceConfig, err := config.GetSpaceEngineConfig()
		if err != nil {
			return nil, err
		}
		return toJSONValue(spaceConfig)
	case ModuleNotifications:
		notificationConfig, err := config.GetNotificationConfig()
		if err != nil {
			return nil, err
		}
		return toJSONValue(notificationConfig)
	case ModuleScanTemplates:
		return exportScanTemplates()
	default:
		return nil, fmt.Errorf("不支持的配置模块: %s", id)
	}
}

func exportScanTemplates() ([]exportedScanTemplate, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, fmt.Errorf("获取数据库连接失败: %w", err)
	}

	var records []database.ScanTemplate
	if err := db.Order("created_at asc").Find(&records).Error; err != nil {
		return nil, fmt.Errorf("读取扫描模板失败: %w", err)
	}

	templates := make([]exportedScanTemplate, 0, len(records))
	for _, record := range records {
		item := exportedScanTemplate{
			Name:        record.Name,
			Description: record.Description,
			TaskType:    record.TaskType,
		}
		if trimmed := trimJSON(record.Config); trimmed != "" {
			item.Config = json.RawMessage(trimmed)
		}
		templates = append(templates, item)
	}
	return templates, nil
}

func toJSONValue(value any) (any, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var result any
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func trimJSON(value string) string {
	if value == "" || value == "null" {
		return ""
	}
	return value
}
